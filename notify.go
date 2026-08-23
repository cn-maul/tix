package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// ====================================================================
// 统一推送模块
//
// 所有需要对外发通知的功能（定期待处理工单汇总、告警等）共用同一入口：
//   msg := &NotifyMessage{Title: "...", Content: "..."}
//   results := a.notify.Send(msg)
//
// 新增渠道只需实现 notifyChannel 并注册到 notifier.channels。
// 渠道配置存于 settings 表（notify_ 前缀，属敏感键，不随公开设置接口下发）。
// ====================================================================

// ---------- 设置键 ----------

const (
	settingPushPlusEnabled = "notify_pushplus_enabled" // "1"/"0"
	settingPushPlusToken   = "notify_pushplus_token"
	settingPushPlusTopic   = "notify_pushplus_topic" // 可选群组编码，留空发给本人
)

// publicSettingKeys 公开设置白名单：/api/settings 无需登录，
// 仅允许下发非敏感配置；其余（如 notify_* 的 Token）只能走管理端接口。
var publicSettingKeys = map[string]bool{
	"site_name": true,
}

// ---------- 统一消息模型 ----------

// NotifyMessage 渠道无关的推送消息。
type NotifyMessage struct {
	Title    string // 标题
	Content  string // 正文
	Template string // 正文模板：txt/html/markdown/json；空值由渠道取默认
}

// NotifyResult 单渠道发送结果。
type NotifyResult struct {
	Channel string `json:"channel"`
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
}

// ---------- 渠道抽象与注册 ----------

// notifyChannel 推送渠道接口。
type notifyChannel interface {
	name() string
	// configured 判断该渠道是否已启用且配置完整。
	configured(db *sql.DB) (bool, error)
	// send 发送消息；返回 error 表示发送失败。
	send(db *sql.DB, msg *NotifyMessage) error
}

// notifier 统一推送入口。
type notifier struct {
	channels []notifyChannel
	hc       *http.Client
}

func newNotifier() *notifier {
	n := &notifier{
		hc: &http.Client{Timeout: 15 * time.Second},
	}
	n.channels = []notifyChannel{
		&pushPlusChannel{hc: n.hc},
		// 后续新渠道在此追加
	}
	return n
}

// Send 通过所有已启用渠道发送消息，返回各渠道结果（调用方决定如何呈现/记录）。
func (n *notifier) Send(db *sql.DB, msg *NotifyMessage) []NotifyResult {
	var results []NotifyResult
	for _, ch := range n.channels {
		ok, err := ch.configured(db)
		if err != nil {
			log.Printf("[推送] %s 读取配置失败: %v", ch.name(), err)
			results = append(results, NotifyResult{Channel: ch.name(), OK: false, Error: "读取配置失败"})
			continue
		}
		if !ok {
			continue
		}
		err = ch.send(db, msg)
		if err != nil {
			log.Printf("[推送] %s 发送失败: %v", ch.name(), err)
			results = append(results, NotifyResult{Channel: ch.name(), OK: false, Error: err.Error()})
			continue
		}
		log.Printf("[推送] %s 已发送：%s", ch.name(), msg.Title)
		results = append(results, NotifyResult{Channel: ch.name(), OK: true})
	}
	return results
}

// hasConfiguredChannel 是否存在已启用的渠道（用于测试接口的前置校验）。
func (n *notifier) hasConfiguredChannel(db *sql.DB) bool {
	for _, ch := range n.channels {
		if ok, err := ch.configured(db); err == nil && ok {
			return true
		}
	}
	return false
}

// ====================================================================
// PushPlus 渠道
// 文档：https://www.pushplus.plus/ （POST JSON，code=200 视为成功）
// ====================================================================

// pushPlusSendURL 独立成变量，便于单测替换为本地 httptest 服务。
var pushPlusSendURL = "https://www.pushplus.plus/send"

type pushPlusChannel struct {
	hc *http.Client
}

func (p *pushPlusChannel) name() string { return "pushplus" }

func (p *pushPlusChannel) configured(db *sql.DB) (bool, error) {
	cfg, err := loadNotifySettings(db)
	if err != nil {
		return false, err
	}
	return cfg.Enabled && cfg.Token != "", nil
}

func (p *pushPlusChannel) send(db *sql.DB, msg *NotifyMessage) error {
	cfg, err := loadNotifySettings(db)
	if err != nil {
		return err
	}
	if cfg.Token == "" {
		return errors.New("未配置 Token")
	}

	template := msg.Template
	if template == "" {
		template = "txt"
	}
	payload := map[string]string{
		"token":    cfg.Token,
		"title":    msg.Title,
		"content":  msg.Content,
		"template": template,
	}
	if cfg.Topic != "" {
		payload["topic"] = cfg.Topic
	}
	body, _ := json.Marshal(payload)

	hc := p.hc
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := hc.Post(pushPlusSendURL, "application/json; charset=utf-8", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("请求 pushplus 失败: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("pushplus 返回 HTTP %d: %s", resp.StatusCode, truncateForLog(string(data)))
	}
	var pr struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(data, &pr); err != nil {
		return fmt.Errorf("解析响应失败: %s", truncateForLog(string(data)))
	}
	if pr.Code != 200 {
		return fmt.Errorf("pushplus 错误 code=%d: %s", pr.Code, pr.Msg)
	}
	return nil
}

// truncateForLog 截断过长响应，避免日志爆炸。
func truncateForLog(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}

// ---------- 配置读写 ----------

type notifySettings struct {
	Enabled bool
	Token   string
	Topic   string
}

// loadNotifySettings 从 settings 表读取推送配置（每次实时读取，改动即时生效）。
func loadNotifySettings(db *sql.DB) (*notifySettings, error) {
	m, err := getAllSettings(db)
	if err != nil {
		return nil, err
	}
	return &notifySettings{
		Enabled: m[settingPushPlusEnabled] == "1",
		Token:   strings.TrimSpace(m[settingPushPlusToken]),
		Topic:   strings.TrimSpace(m[settingPushPlusTopic]),
	}, nil
}

// maskToken 脱敏展示：保留前 2 位与后 4 位，其余以 **** 代替。
func maskToken(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) <= 8 {
		return "****"
	}
	return string(r[:2]) + "****" + string(r[len(r)-4:])
}

// app.notify 挂在应用上的统一推送实例（渠道注册一次，HTTP 客户端复用）。

// ====================================================================
// 管理端接口（仅管理员）
// ====================================================================

// apiNotifyConfigGet GET /api/notify/config —— 查看推送配置（Token 仅返回脱敏形式）。
func (a *app) apiNotifyConfigGet(w http.ResponseWriter, r *http.Request) {
	if a.requireAdmin(w, r) == nil {
		return
	}
	cfg, err := loadNotifySettings(a.db)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "读取推送配置失败")
		return
	}
	jsonResp(w, http.StatusOK, map[string]any{"data": map[string]any{
		"enabled":      boolToInt(cfg.Enabled),
		"token_set":    cfg.Token != "",
		"token_masked": maskToken(cfg.Token),
		"topic":        cfg.Topic,
	}})
}

// apiNotifyConfigUpdate PUT /api/notify/config —— 部分更新推送配置。
// token 字段出现即生效：传空串清除，不传保持不变。
func (a *app) apiNotifyConfigUpdate(w http.ResponseWriter, r *http.Request) {
	if a.requireAdmin(w, r) == nil {
		return
	}
	var body struct {
		Enabled *int    `json:"enabled"`
		Token   *string `json:"token"`
		Topic   *string `json:"topic"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		jsonError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}

	if body.Enabled != nil {
		if *body.Enabled != 0 && *body.Enabled != 1 {
			jsonError(w, http.StatusBadRequest, "enabled 仅支持 0/1")
			return
		}
		if err := setSetting(a.db, settingPushPlusEnabled, fmt.Sprint(*body.Enabled)); err != nil {
			jsonError(w, http.StatusInternalServerError, "保存推送配置失败")
			return
		}
	}
	if body.Token != nil {
		token := strings.TrimSpace(*body.Token)
		if len(token) > 256 {
			jsonError(w, http.StatusBadRequest, "Token 过长")
			return
		}
		if err := setSetting(a.db, settingPushPlusToken, token); err != nil {
			jsonError(w, http.StatusInternalServerError, "保存推送配置失败")
			return
		}
	}
	if body.Topic != nil {
		topic := strings.TrimSpace(*body.Topic)
		if len([]rune(topic)) > 64 {
			jsonError(w, http.StatusBadRequest, "群组编码过长（最多 64 字符）")
			return
		}
		if err := setSetting(a.db, settingPushPlusTopic, topic); err != nil {
			jsonError(w, http.StatusInternalServerError, "保存推送配置失败")
			return
		}
	}
	a.apiNotifyConfigGet(w, r) // 返回保存后的最新配置
}

// apiNotifyTest POST /api/notify/test —— 向已启用渠道发送一条测试消息。
func (a *app) apiNotifyTest(w http.ResponseWriter, r *http.Request) {
	if a.requireAdmin(w, r) == nil {
		return
	}
	if !a.notify.hasConfiguredChannel(a.db) {
		jsonError(w, http.StatusBadRequest, "请先启用推送并填写 Token")
		return
	}
	msg := &NotifyMessage{
		Title:    "tix 推送测试",
		Content:  fmt.Sprintf("这是一条来自 tix 工单系统的测试消息。\n发送时间：%s\n收到即表示推送通道正常。", nowStr()),
		Template: "txt",
	}
	results := a.notify.Send(a.db, msg)
	jsonResp(w, http.StatusOK, map[string]any{"data": map[string]any{"results": results}})
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
