package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookie = "tix_session"
	sessionTTL    = 7 * 24 * time.Hour
)

// sessionEntry 存储会话对应的用户信息。
type sessionEntry struct {
	userID    int64
	username  string
	role      string
	expiresAt time.Time
}

// authStore 内存会话表：登录后签发随机 token，服务重启即全部失效（需重新登录）。
type authStore struct {
	mu       sync.Mutex
	sessions map[string]sessionEntry
}

func newAuthStore() *authStore {
	return &authStore{sessions: map[string]sessionEntry{}}
}

func (s *authStore) create(userID int64, username, role string) string {
	buf := make([]byte, 32)
	_, _ = rand.Read(buf)
	token := hex.EncodeToString(buf)
	s.mu.Lock()
	s.sessions[token] = sessionEntry{
		userID:    userID,
		username:  username,
		role:      role,
		expiresAt: time.Now().Add(sessionTTL),
	}
	if len(s.sessions) > 512 {
		now := time.Now()
		for k, v := range s.sessions {
			if now.After(v.expiresAt) {
				delete(s.sessions, k)
			}
		}
	}
	s.mu.Unlock()
	return token
}

func (s *authStore) get(token string) *sessionEntry {
	if token == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.sessions[token]
	if !ok {
		return nil
	}
	if time.Now().After(e.expiresAt) {
		delete(s.sessions, token)
		return nil
	}
	return &e
}

func (s *authStore) revoke(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

// revokeUser 吊销某用户的全部会话。修改密码后调用，
// 强制该用户所有已登录的会话（含发起修改的当前会话）下线重新登录。
func (s *authStore) revokeUser(userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.sessions {
		if v.userID == userID {
			delete(s.sessions, k)
		}
	}
}

// requireAuth 校验会话 cookie；未通过时写 401 并返回 nil。
func (a *app) requireAuth(w http.ResponseWriter, r *http.Request) *sessionEntry {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		jsonError(w, http.StatusUnauthorized, "未登录或登录已过期")
		return nil
	}
	sess := a.auth.get(c.Value)
	if sess == nil {
		jsonError(w, http.StatusUnauthorized, "未登录或登录已过期")
		return nil
	}
	return sess
}

// requireAdmin 要求管理员身份；非管理员返回 403。
func (a *app) requireAdmin(w http.ResponseWriter, r *http.Request) *sessionEntry {
	sess := a.requireAuth(w, r)
	if sess == nil {
		return nil
	}
	if sess.role != "admin" {
		jsonError(w, http.StatusForbidden, "需要管理员权限")
		return nil
	}
	return sess
}

func setSessionCookie(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})
}

// --------------------------------------------------------------------
// 密码哈希
// --------------------------------------------------------------------

// hashPassword 生成 bcrypt 哈希（数据库中只存哈希，不存明文）。
func hashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// checkPassword 校验密码。兼容旧版本明文口令：命中明文时 needsRehash=true，
// 调用方应在登录成功后将其升级为 bcrypt 哈希。
func checkPassword(stored, input string) (ok bool, needsRehash bool) {
	if strings.HasPrefix(stored, "$2") {
		err := bcrypt.CompareHashAndPassword([]byte(stored), []byte(input))
		return err == nil, false
	}
	// 明文（历史数据）：恒定时间比较，避免时序侧信道
	if subtle.ConstantTimeCompare([]byte(stored), []byte(input)) == 1 {
		return true, true
	}
	return false, false
}

// --------------------------------------------------------------------
// 登录 / 登出 / 状态
// --------------------------------------------------------------------

func (a *app) apiLogin(w http.ResponseWriter, r *http.Request) {
	// 登录限流：同IP每分钟最多10次，缓解暴力破解
	if !loginLimiter.allow(a.clientIP(r)) {
		jsonError(w, http.StatusTooManyRequests, "尝试过于频繁，请稍后再试")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if req.Username == "" || req.Password == "" {
		jsonError(w, http.StatusBadRequest, "用户名和密码不能为空")
		return
	}

	user, err := getUserByUsername(a.db, req.Username)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "查询用户失败")
		return
	}
	if user == nil {
		jsonError(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}

	pw, _ := getUserPassword(a.db, req.Username)
	ok, needsRehash := checkPassword(pw, req.Password)
	if !ok {
		jsonError(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	// 旧版明文密码在登录成功后透明升级为 bcrypt
	if needsRehash {
		if err := updateUserPassword(a.db, user.ID, req.Password); err != nil {
			log.Printf("升级用户 %d 密码哈希失败: %v", user.ID, err)
		} else {
			log.Printf("用户 %s 的明文密码已升级为 bcrypt 哈希", user.Username)
		}
	}

	token := a.auth.create(user.ID, user.Username, user.Role)
	setSessionCookie(w, token, int(sessionTTL.Seconds()))
	jsonResp(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"ok":   true,
			"user": user,
		},
	})
}

func (a *app) apiLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		a.auth.revoke(c.Value)
	}
	setSessionCookie(w, "", -1)
	jsonResp(w, http.StatusOK, map[string]any{"data": map[string]any{"ok": true}})
}

func (a *app) apiAuthStatus(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		jsonResp(w, http.StatusOK, map[string]any{"data": map[string]any{"ok": false}})
		return
	}
	sess := a.auth.get(c.Value)
	if sess == nil {
		jsonResp(w, http.StatusOK, map[string]any{"data": map[string]any{"ok": false}})
		return
	}
	user, _ := getUserByID(a.db, sess.userID)
	jsonResp(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"ok":   true,
			"user": user,
		},
	})
}

// --------------------------------------------------------------------
// 用户管理（仅管理员）
// --------------------------------------------------------------------

func (a *app) apiUserList(w http.ResponseWriter, r *http.Request) {
	if a.requireAdmin(w, r) == nil {
		return
	}
	users, err := listUsers(a.db)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "查询用户失败")
		return
	}
	jsonResp(w, http.StatusOK, map[string]any{"data": users})
}

func (a *app) apiUserCreate(w http.ResponseWriter, r *http.Request) {
	if a.requireAdmin(w, r) == nil {
		return
	}
	var req struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		Role        string `json:"role"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if req.Username == "" || req.Password == "" || req.DisplayName == "" {
		jsonError(w, http.StatusBadRequest, "用户名、密码和显示名不能为空")
		return
	}
	if errMsg := validateUsername(req.Username); errMsg != "" {
		jsonError(w, http.StatusBadRequest, errMsg)
		return
	}
	if len(req.Password) < 6 {
		jsonError(w, http.StatusBadRequest, "密码长度须至少6位")
		return
	}
	if len([]rune(req.DisplayName)) > 32 {
		jsonError(w, http.StatusBadRequest, "显示名过长（最多32字符）")
		return
	}
	if req.Role != "admin" && req.Role != "operator" {
		req.Role = "operator"
	}
	id, err := createUser(a.db, req.Username, req.Password, req.DisplayName, req.Role)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			jsonError(w, http.StatusConflict, "用户名已存在")
			return
		}
		jsonError(w, http.StatusInternalServerError, "创建用户失败")
		return
	}
	jsonResp(w, http.StatusCreated, map[string]any{"data": map[string]any{"id": id}})
}

func (a *app) apiUserUpdate(w http.ResponseWriter, r *http.Request) {
	admin := a.requireAdmin(w, r)
	if admin == nil {
		return
	}
	id, ok := parseID(r)
	if !ok {
		jsonError(w, http.StatusBadRequest, "无效的用户 ID")
		return
	}
	target, err := getUserByID(a.db, id)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "查询用户失败")
		return
	}
	if target == nil {
		http.NotFound(w, r)
		return
	}
	var req struct {
		DisplayName string `json:"display_name"`
		Role        string `json:"role"`
		Password    string `json:"password,omitempty"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if req.Role != "admin" && req.Role != "operator" {
		req.Role = "operator"
	}
	// 不允许修改自己的角色（显示名和密码可以自助修改）
	if id == admin.userID && req.Role != target.Role {
		jsonError(w, http.StatusBadRequest, "不能修改自己的角色")
		return
	}
	if len([]rune(req.DisplayName)) > 32 {
		jsonError(w, http.StatusBadRequest, "显示名过长（最多32字符）")
		return
	}
	if err := updateUserProfile(a.db, id, req.DisplayName, req.Role); err != nil {
		jsonError(w, http.StatusInternalServerError, "更新用户失败")
		return
	}
	if req.Password != "" {
		if len(req.Password) < 6 {
			jsonError(w, http.StatusBadRequest, "密码长度须至少6位")
			return
		}
		if err := updateUserPassword(a.db, id, req.Password); err != nil {
			jsonError(w, http.StatusInternalServerError, "更新密码失败")
			return
		}
		// 改密后强制该用户全部会话下线（含管理员改他人、用户自助改密两种情形）
		a.auth.revokeUser(id)
	}
	jsonResp(w, http.StatusOK, map[string]any{"data": map[string]any{"ok": true}})
}

func (a *app) apiUserDelete(w http.ResponseWriter, r *http.Request) {
	admin := a.requireAdmin(w, r)
	if admin == nil {
		return
	}
	id, ok := parseID(r)
	if !ok {
		jsonError(w, http.StatusBadRequest, "无效的用户 ID")
		return
	}
	if id == admin.userID {
		jsonError(w, http.StatusBadRequest, "不能删除自己")
		return
	}
	if err := deleteUser(a.db, id); err != nil {
		jsonError(w, http.StatusInternalServerError, "删除用户失败")
		return
	}
	jsonResp(w, http.StatusOK, map[string]any{"data": map[string]any{"ok": true}})
}

// --------------------------------------------------------------------
// 设置
// --------------------------------------------------------------------

func (a *app) apiSettingsGet(w http.ResponseWriter, r *http.Request) {
	all, err := getAllSettings(a.db)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "读取设置失败")
		return
	}
	// /api/settings 为公开接口：仅下发白名单内的非敏感键（推送 Token 等走 /api/notify/config）。
	settings := make(map[string]string, len(all))
	for k, v := range all {
		if publicSettingKeys[k] {
			settings[k] = v
		}
	}
	jsonResp(w, http.StatusOK, map[string]any{"data": settings})
}

func (a *app) apiSettingsUpdate(w http.ResponseWriter, r *http.Request) {
	if a.requireAdmin(w, r) == nil {
		return
	}
	var req map[string]string
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	for k, v := range req {
		if err := setSetting(a.db, k, v); err != nil {
			jsonError(w, http.StatusInternalServerError, "保存设置失败")
			return
		}
	}
	jsonResp(w, http.StatusOK, map[string]any{"data": map[string]any{"ok": true}})
}
