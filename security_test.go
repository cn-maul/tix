package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ======================================================================
// 登录限流
// ======================================================================

func TestLoginRateLimit(t *testing.T) {
	a := newTestApp(t)
	// 换用小阈值限流器，避免影响其他用例
	a.loginLimiter = newRateLimiter(3, time.Minute)
	h := a.authMiddleware(a.routes())
	if _, err := createUser(a.db, "limuser", "secret123", "限流用户", "operator"); err != nil {
		t.Fatalf("createUser: %v", err)
	}

	codes := make([]int, 0, 5)
	for i := 0; i < 5; i++ {
		rr := postJSON(t, h, "/api/login", map[string]string{"username": "limuser", "password": "wrong"})
		codes = append(codes, rr.Code)
	}
	if codes[0] != http.StatusUnauthorized || codes[1] != http.StatusUnauthorized || codes[2] != http.StatusUnauthorized {
		t.Fatalf("first 3 attempts should be 401, got %v", codes)
	}
	if codes[3] != http.StatusTooManyRequests || codes[4] != http.StatusTooManyRequests {
		t.Fatalf("attempts beyond limit should be 429, got %v", codes)
	}
}

// ======================================================================
// 密码哈希：创建即哈希 + 存量明文由启动迁移升级
// ======================================================================

func TestPasswordStoredHashed(t *testing.T) {
	a := newTestApp(t)
	if _, err := createUser(a.db, "hashuser", "secret123", "哈希用户", "operator"); err != nil {
		t.Fatalf("createUser: %v", err)
	}
	_, stored, _ := getUserAuth(a.db, "hashuser")
	if !strings.HasPrefix(stored, "$2") {
		t.Fatalf("password should be bcrypt-hashed, got %.10q", stored)
	}
	if !checkPassword(stored, "secret123") {
		t.Fatal("checkPassword should match correct password")
	}
	if checkPassword(stored, "wrong") {
		t.Fatal("wrong password should not match")
	}
}

func TestLegacyPlaintextLoginAfterMigration(t *testing.T) {
	a := newTestApp(t)
	a.auth = newAuthStore()
	h := a.authMiddleware(a.routes())
	// 模拟旧版本遗留的明文密码行（newTestApp 已跑过一次迁移，此处补跑覆盖新行）
	if _, err := a.db.Exec(
		"INSERT INTO users (username, password, display_name, role, created_at) VALUES (?, ?, ?, ?, ?)",
		"legacy", "oldplain", "旧用户", "operator", nowStr()); err != nil {
		t.Fatalf("seed legacy user: %v", err)
	}
	if err := migrateDB(a.db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// 启动迁移已将明文升级为 bcrypt，旧密码可正常登录
	_, stored, _ := getUserAuth(a.db, "legacy")
	if !strings.HasPrefix(stored, "$2") {
		t.Fatalf("legacy plaintext should be migrated to bcrypt at startup, got %.10q", stored)
	}
	rr := postJSON(t, h, "/api/login", map[string]string{"username": "legacy", "password": "oldplain"})
	requireStatus(t, rr, http.StatusOK)

	// 错误密码仍 401
	rr = postJSON(t, h, "/api/login", map[string]string{"username": "legacy", "password": "nope"})
	requireStatus(t, rr, http.StatusUnauthorized)
}

func TestMigratePlaintextPasswordsIdempotent(t *testing.T) {
	a := newTestApp(t)
	for _, u := range []struct{ name, pw string }{{"p1", "alpha123"}, {"p2", "beta123"}} {
		if _, err := a.db.Exec(
			"INSERT INTO users (username, password, display_name, role, created_at) VALUES (?, ?, ?, ?, ?)",
			u.name, u.pw, u.name, "operator", nowStr()); err != nil {
			t.Fatalf("seed %s: %v", u.name, err)
		}
	}
	if err := migrateDB(a.db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for _, u := range []struct{ name, pw string }{{"p1", "alpha123"}, {"p2", "beta123"}} {
		_, stored, _ := getUserAuth(a.db, u.name)
		if !strings.HasPrefix(stored, "$2") {
			t.Fatalf("%s not migrated: %.10q", u.name, stored)
		}
		if !checkPassword(stored, u.pw) {
			t.Fatalf("%s password broken after migration", u.name)
		}
	}
	// 幂等：再次迁移不报错且哈希不变
	_, before, _ := getUserAuth(a.db, "p1")
	if err := migrateDB(a.db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	_, after, _ := getUserAuth(a.db, "p1")
	if before != after {
		t.Fatal("re-migrate should not rewrite hashes")
	}
}

// ======================================================================
// 用户自助更新：可改自己的显示名/密码，不可改角色
// ======================================================================

type userData struct {
	Data []User `json:"data"`
}

func loginAs(t *testing.T, h http.Handler, username, password string) *http.Cookie {
	t.Helper()
	rr := postJSON(t, h, "/api/login", map[string]string{"username": username, "password": password})
	requireStatus(t, rr, http.StatusOK)
	for _, c := range rr.Result().Cookies() {
		if c.Name == sessionCookie {
			return c
		}
	}
	t.Fatal("no session cookie")
	return nil
}

func TestSelfUpdateAllowedExceptRole(t *testing.T) {
	a := newTestApp(t)
	a.auth = newAuthStore()
	h := a.authMiddleware(a.routes())
	if _, err := createUser(a.db, "selfadmin", "secret123", "管理员", "admin"); err != nil {
		t.Fatalf("createUser: %v", err)
	}
	c := loginAs(t, h, "selfadmin", "secret123")

	// 查自己的 id
	rr := reqWithCookie(t, h, http.MethodGet, "/api/users", nil, c)
	requireStatus(t, rr, http.StatusOK)
	var ul userData
	json.NewDecoder(rr.Body).Decode(&ul)
	var selfID int64
	for _, u := range ul.Data {
		if u.Username == "selfadmin" {
			selfID = u.ID
		}
	}
	if selfID == 0 {
		t.Fatal("self user not found")
	}

	// 改自己的显示名 + 密码 → 200，新密码生效；旧会话被吊销
	rr = reqWithCookie(t, h, http.MethodPut, "/api/users/"+intToString(selfID),
		map[string]any{"display_name": "新名字", "role": "admin", "password": "newpass66"}, c)
	requireStatus(t, rr, http.StatusOK)
	rr = reqWithCookie(t, h, http.MethodGet, "/api/users", nil, c)
	requireStatus(t, rr, http.StatusUnauthorized)

	// 用新密码重新登录，后续用新会话
	c = loginAs(t, h, "selfadmin", "newpass66")

	// 改自己的角色 → 400
	rr = reqWithCookie(t, h, http.MethodPut, "/api/users/"+intToString(selfID),
		map[string]any{"display_name": "新名字", "role": "operator"}, c)
	requireStatus(t, rr, http.StatusBadRequest)

	// 角色保持不变
	u, _ := getUserByID(a.db, selfID)
	if u.Role != "admin" || u.DisplayName != "新名字" {
		t.Fatalf("profile unexpected: %+v", u)
	}

	// 管理员改他人不受限制
	if _, err := createUser(a.db, "other", "secret123", "别人", "operator"); err != nil {
		t.Fatalf("createUser: %v", err)
	}
	rows, err := a.db.Query("SELECT id FROM users WHERE username='other'")
	if err != nil {
		t.Fatal(err)
	}
	var otherID int64
	for rows.Next() {
		_ = rows.Scan(&otherID)
	}
	rows.Close()
	rr = reqWithCookie(t, h, http.MethodPut, "/api/users/"+intToString(otherID),
		map[string]any{"display_name": "别人改名", "role": "admin"}, c)
	requireStatus(t, rr, http.StatusOK)
}

// ======================================================================
// 改密后强制下线：吊销该用户全部会话
// ======================================================================

func TestPasswordChangeRevokesSessions(t *testing.T) {
	a := newTestApp(t)
	a.auth = newAuthStore()
	h := a.authMiddleware(a.routes())
	if _, err := createUser(a.db, "revadmin", "secret123", "改密管理员", "admin"); err != nil {
		t.Fatalf("createUser: %v", err)
	}
	if _, err := createUser(a.db, "revuser", "secret123", "普通用户", "operator"); err != nil {
		t.Fatalf("createUser: %v", err)
	}

	cA := loginAs(t, h, "revadmin", "secret123")
	cB := loginAs(t, h, "revuser", "secret123")

	// 找出两个用户的 id
	rr := reqWithCookie(t, h, http.MethodGet, "/api/users", nil, cA)
	requireStatus(t, rr, http.StatusOK)
	var ul struct {
		Data []User `json:"data"`
	}
	json.NewDecoder(rr.Body).Decode(&ul)
	var adminID, userID int64
	for _, u := range ul.Data {
		switch u.Username {
		case "revadmin":
			adminID = u.ID
		case "revuser":
			userID = u.ID
		}
	}
	if adminID == 0 || userID == 0 {
		t.Fatalf("users not found: %+v", ul.Data)
	}

	// B 的会话当前有效
	rr = reqWithCookie(t, h, http.MethodGet, "/api/stats", nil, cB)
	requireStatus(t, rr, http.StatusOK)

	// 管理员改 B 的密码 → B 的旧会话立即失效
	rr = reqWithCookie(t, h, http.MethodPut, "/api/users/"+intToString(userID),
		map[string]any{"display_name": "普通用户", "role": "operator", "password": "newpass99"}, cA)
	requireStatus(t, rr, http.StatusOK)
	rr = reqWithCookie(t, h, http.MethodGet, "/api/stats", nil, cB)
	requireStatus(t, rr, http.StatusUnauthorized)

	// 新密码可登录，新会话有效
	cB2 := loginAs(t, h, "revuser", "newpass99")
	rr = reqWithCookie(t, h, http.MethodGet, "/api/stats", nil, cB2)
	requireStatus(t, rr, http.StatusOK)

	// 自己改自己的密码 → 当前会话同样被吊销
	rr = reqWithCookie(t, h, http.MethodPut, "/api/users/"+intToString(adminID),
		map[string]any{"display_name": "改密管理员", "role": "admin", "password": "adminnew99"}, cA)
	requireStatus(t, rr, http.StatusOK)
	rr = reqWithCookie(t, h, http.MethodGet, "/api/users", nil, cA)
	requireStatus(t, rr, http.StatusUnauthorized)

	// 改显示名（不改密）不应吊销会话
	cA2 := loginAs(t, h, "revadmin", "adminnew99")
	rr = reqWithCookie(t, h, http.MethodPut, "/api/users/"+intToString(adminID),
		map[string]any{"display_name": "改名不改密", "role": "admin"}, cA2)
	requireStatus(t, rr, http.StatusOK)
	rr = reqWithCookie(t, h, http.MethodGet, "/api/users", nil, cA2)
	requireStatus(t, rr, http.StatusOK)
}

// ======================================================================
// 工单编号格式 T-YYYYMMDD-NNNN
// ======================================================================

func TestTicketNumberFormat(t *testing.T) {
	cases := []struct {
		in   Ticket
		want string
	}{
		{Ticket{ID: 1, CreatedAt: "2026-08-18 10:00:00"}, "T-20260818-0001"},
		{Ticket{ID: 42, CreatedAt: "2026-01-02 09:05:00"}, "T-20260102-0042"},
	}
	for _, tc := range cases {
		if got := ticketNumber(&tc.in); got != tc.want {
			t.Fatalf("ticketNumber()=%q want %q", got, tc.want)
		}
	}
}

// ======================================================================
// 限流器 key 清扫（防内存无界增长）
// ======================================================================

func TestRateLimiterSweep(t *testing.T) {
	rl := newRateLimiter(1000, time.Minute)
	// 塞满超过阈值的过期 key
	past := time.Now().Add(-2 * time.Minute)
	for i := 0; i < maxRateKeys+10; i++ {
		rl.requests[string(rune('a'+i%26))+time.Now().Format("150405.000000000")+string(rune(i))] = []time.Time{past}
	}
	if len(rl.requests) <= maxRateKeys {
		t.Fatal("setup: expected oversized map")
	}
	// 新请求应触发清扫
	if !rl.allow("fresh-key") {
		t.Fatal("fresh key should pass")
	}
	if len(rl.requests) > maxRateKeys {
		t.Fatalf("sweep did not shrink map: %d keys left", len(rl.requests))
	}
}

// ======================================================================
// 删除用户后其已有会话立即失效（requireAuth 回查数据库 + revokeUser 双保险）
// ======================================================================

func TestDeleteUserRevokesSessions(t *testing.T) {
	a := newTestApp(t)
	a.auth = newAuthStore()
	h := a.authMiddleware(a.routes())
	if _, err := createUser(a.db, "deladmin", "secret123", "删除管理员", "admin"); err != nil {
		t.Fatalf("createUser: %v", err)
	}
	if _, err := createUser(a.db, "victim", "secret123", "被删用户", "operator"); err != nil {
		t.Fatalf("createUser: %v", err)
	}
	cA := loginAs(t, h, "deladmin", "secret123")
	cV := loginAs(t, h, "victim", "secret123")

	// 删除前会话有效
	rr := reqWithCookie(t, h, http.MethodGet, "/api/stats", nil, cV)
	requireStatus(t, rr, http.StatusOK)

	// 管理员删除 victim
	rr = reqWithCookie(t, h, http.MethodGet, "/api/users", nil, cA)
	requireStatus(t, rr, http.StatusOK)
	var ul struct {
		Data []User `json:"data"`
	}
	json.NewDecoder(rr.Body).Decode(&ul)
	var victimID int64
	for _, u := range ul.Data {
		if u.Username == "victim" {
			victimID = u.ID
		}
	}
	if victimID == 0 {
		t.Fatal("victim not found")
	}
	req := httptest.NewRequest(http.MethodDelete, "/api/users/"+intToString(victimID), nil)
	req.AddCookie(cA)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	requireStatus(t, rr, http.StatusOK)

	// 删除后：旧会话立即失效
	rr = reqWithCookie(t, h, http.MethodGet, "/api/stats", nil, cV)
	requireStatus(t, rr, http.StatusUnauthorized)
	// auth/status 也不应再返回 ok:true
	rr = reqWithCookie(t, h, http.MethodGet, "/api/auth/status", nil, cV)
	requireStatus(t, rr, http.StatusOK)
	var st struct {
		Data struct {
			OK bool `json:"ok"`
		} `json:"data"`
	}
	json.NewDecoder(rr.Body).Decode(&st)
	if st.Data.OK {
		t.Fatal("deleted user auth/status should be ok=false")
	}
}

// ======================================================================
// 角色降级立即生效：降级后的旧会话不再拥有管理员权限
// ======================================================================

func TestRoleDemotionTakesEffect(t *testing.T) {
	a := newTestApp(t)
	a.auth = newAuthStore()
	h := a.authMiddleware(a.routes())
	if _, err := createUser(a.db, "boss", "secret123", "管理员", "admin"); err != nil {
		t.Fatalf("createUser boss: %v", err)
	}
	if _, err := createUser(a.db, "helper", "secret123", "副手", "admin"); err != nil {
		t.Fatalf("createUser helper: %v", err)
	}
	cBoss := loginAs(t, h, "boss", "secret123")
	cHelper := loginAs(t, h, "helper", "secret123")

	rr := reqWithCookie(t, h, http.MethodGet, "/api/users", nil, cBoss)
	requireStatus(t, rr, http.StatusOK)
	var ul struct {
		Data []User `json:"data"`
	}
	json.NewDecoder(rr.Body).Decode(&ul)
	var helperID int64
	for _, u := range ul.Data {
		if u.Username == "helper" {
			helperID = u.ID
		}
	}
	if helperID == 0 {
		t.Fatal("helper not found")
	}

	// 降级前：helper 会话可访问管理接口
	rr = reqWithCookie(t, h, http.MethodGet, "/api/users", nil, cHelper)
	requireStatus(t, rr, http.StatusOK)

	// 管理员把 helper 降级为 operator（不改密码、不显式吊销会话）
	rr = reqWithCookie(t, h, http.MethodPut, "/api/users/"+intToString(helperID),
		map[string]any{"display_name": "副手", "role": "operator"}, cBoss)
	requireStatus(t, rr, http.StatusOK)

	// 降级立即生效：用户列表对所有登录用户开放（供指派取人），但
	// 管理员写操作（如创建用户）→ 403；普通接口仍可用
	rr = reqWithCookie(t, h, http.MethodGet, "/api/users", nil, cHelper)
	requireStatus(t, rr, http.StatusOK)
	rr = reqWithCookie(t, h, http.MethodPost, "/api/users",
		map[string]any{"username": "nope", "password": "secret123", "display_name": "x", "role": "operator"}, cHelper)
	requireStatus(t, rr, http.StatusForbidden)
	rr = reqWithCookie(t, h, http.MethodGet, "/api/stats", nil, cHelper)
	requireStatus(t, rr, http.StatusOK)
}

// ======================================================================
// 设置接口键白名单：非白名单键拒绝，site_name 长度限制
// ======================================================================

func TestSettingsWhitelist(t *testing.T) {
	a := newTestApp(t)
	a.auth = newAuthStore()
	h := a.authMiddleware(a.routes())
	if _, err := createUser(a.db, "setadmin", "secret123", "设置管理员", "admin"); err != nil {
		t.Fatalf("createUser: %v", err)
	}
	c := loginAs(t, h, "setadmin", "secret123")

	// 白名单外键 → 400
	rr := reqWithCookie(t, h, http.MethodPut, "/api/settings", map[string]string{"unknown_key": "x"}, c)
	requireStatus(t, rr, http.StatusBadRequest)
	// 白名单内键 → 200
	rr = reqWithCookie(t, h, http.MethodPut, "/api/settings", map[string]string{"site_name": "运维中心"}, c)
	requireStatus(t, rr, http.StatusOK)
	// site_name 超长 → 400
	rr = reqWithCookie(t, h, http.MethodPut, "/api/settings", map[string]string{"site_name": strings.Repeat("长", 40)}, c)
	requireStatus(t, rr, http.StatusBadRequest)
}

// ======================================================================
// 基础安全响应头
// ======================================================================

func TestSecurityHeaders(t *testing.T) {
	a := newTestApp(t)
	h := securityHeaders(a.routes())
	rr := getJSON(t, h, "/api/health")
	requireStatus(t, rr, http.StatusOK)
	if rr.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing X-Content-Type-Options")
	}
	if rr.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatal("missing X-Frame-Options")
	}
	if rr.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("missing CSP")
	}
}
