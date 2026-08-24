package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// adminCookie 创建管理员账号并登录，返回会话 Cookie。
func adminCookie(t *testing.T, a *app, h http.Handler) *http.Cookie {
	t.Helper()
	if _, err := createUser(a.db, "notifyadmin", "secret123", "推送管理员", "admin"); err != nil {
		t.Fatalf("createUser: %v", err)
	}
	rr := postJSON(t, h, "/api/login", map[string]string{"username": "notifyadmin", "password": "secret123"})
	requireStatus(t, rr, http.StatusOK)
	for _, c := range rr.Result().Cookies() {
		if c.Name == sessionCookie {
			return c
		}
	}
	t.Fatal("no session cookie")
	return nil
}

// reqWithCookie 发送带会话的 JSON 请求。
func reqWithCookie(t *testing.T, h http.Handler, method, target string, body any, c *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var rd io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		rd = bytes.NewReader(data)
	}
	req := httptest.NewRequest(method, target, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c != nil {
		req.AddCookie(c)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

type notifyConfigData struct {
	Data struct {
		PushPlus struct {
			Enabled     int    `json:"enabled"`
			TokenSet    bool   `json:"token_set"`
			TokenMasked string `json:"token_masked"`
			Topic       string `json:"topic"`
		} `json:"pushplus"`
		ServerChan struct {
			Enabled     int    `json:"enabled"`
			SendKeySet  bool   `json:"sendkey_set"`
			SendKeyMask string `json:"sendkey_masked"`
		} `json:"serverchan"`
	} `json:"data"`
}

// ======================================================================
// 推送配置接口鉴权
// ======================================================================

func TestNotifyEndpointsRequireAdmin(t *testing.T) {
	a := newTestApp(t)
	a.auth = newAuthStore()
	h := a.authMiddleware(a.routes())

	if rr := getJSON(t, h, "/api/notify/config"); rr.Code != http.StatusUnauthorized {
		t.Fatalf("unauthed GET config: code=%d", rr.Code)
	}
	if rr := putJSON(t, h, "/api/notify/config", map[string]any{"pushplus": map[string]any{"enabled": 1}}); rr.Code != http.StatusUnauthorized {
		t.Fatalf("unauthed PUT config: code=%d", rr.Code)
	}
	if rr := postJSON(t, h, "/api/notify/test", nil); rr.Code != http.StatusUnauthorized {
		t.Fatalf("unauthed POST test: code=%d", rr.Code)
	}
}

// ======================================================================
// 推送配置读写 + 公开设置不泄露敏感键
// ======================================================================

func TestNotifyConfigFlow(t *testing.T) {
	a := newTestApp(t)
	a.auth = newAuthStore()
	h := a.authMiddleware(a.routes())
	c := adminCookie(t, a, h)

	// 保存 PushPlus 配置
	rr := reqWithCookie(t, h, http.MethodPut, "/api/notify/config",
		map[string]any{"pushplus": map[string]any{"enabled": 1, "token": "abcdefghijklmnop", "topic": "ops组"}}, c)
	requireStatus(t, rr, http.StatusOK)
	var cd notifyConfigData
	json.NewDecoder(rr.Body).Decode(&cd)
	if cd.Data.PushPlus.Enabled != 1 || !cd.Data.PushPlus.TokenSet {
		t.Fatalf("config unexpected: %+v", cd.Data)
	}
	if cd.Data.PushPlus.TokenMasked != "ab****mnop" {
		t.Fatalf("mask unexpected: %q", cd.Data.PushPlus.TokenMasked)
	}
	if cd.Data.PushPlus.Topic != "ops组" {
		t.Fatalf("topic unexpected: %q", cd.Data.PushPlus.Topic)
	}

	// 配置已落库
	cfg, err := loadNotifySettings(a.db)
	if err != nil {
		t.Fatalf("loadNotifySettings: %v", err)
	}
	if !cfg.Enabled || cfg.Token != "abcdefghijklmnop" {
		t.Fatalf("persisted config unexpected: %+v", cfg)
	}

	// 部分更新：仅关开关，Token 应保留
	rr = reqWithCookie(t, h, http.MethodPut, "/api/notify/config",
		map[string]any{"pushplus": map[string]any{"enabled": 0}}, c)
	requireStatus(t, rr, http.StatusOK)
	cfg, _ = loadNotifySettings(a.db)
	if cfg.Token != "abcdefghijklmnop" || cfg.Enabled {
		t.Fatalf("partial update clobbered fields: %+v", cfg)
	}

	// ServerChan 渠道：保存 SendKey，且不影响 PushPlus 配置
	rr = reqWithCookie(t, h, http.MethodPut, "/api/notify/config",
		map[string]any{"serverchan": map[string]any{"enabled": 1, "sendkey": "SCT000111222"}}, c)
	requireStatus(t, rr, http.StatusOK)
	cd = notifyConfigData{}
	json.NewDecoder(rr.Body).Decode(&cd)
	if cd.Data.ServerChan.Enabled != 1 || !cd.Data.ServerChan.SendKeySet || cd.Data.ServerChan.SendKeyMask != "SC****1222" {
		t.Fatalf("serverchan config unexpected: %+v", cd.Data.ServerChan)
	}
	if cd.Data.PushPlus.Enabled != 0 || !cd.Data.PushPlus.TokenSet {
		t.Fatalf("serverchan update clobbered pushplus: %+v", cd.Data.PushPlus)
	}
	scCfg, _ := loadServerChanSettings(a.db)
	if !scCfg.Enabled || scCfg.SendKey != "SCT000111222" {
		t.Fatalf("serverchan persisted unexpected: %+v", scCfg)
	}

	// 显式清空 SendKey
	rr = reqWithCookie(t, h, http.MethodPut, "/api/notify/config",
		map[string]any{"serverchan": map[string]any{"sendkey": ""}}, c)
	requireStatus(t, rr, http.StatusOK)
	cd = notifyConfigData{}
	json.NewDecoder(rr.Body).Decode(&cd)
	if cd.Data.ServerChan.SendKeySet {
		t.Fatal("sendkey should be cleared")
	}

	// 非法 enabled → 400
	rr = reqWithCookie(t, h, http.MethodPut, "/api/notify/config",
		map[string]any{"pushplus": map[string]any{"enabled": 2}}, c)
	requireStatus(t, rr, http.StatusBadRequest)

	// 公开设置接口不得下发 notify_* 键（未登录即可访问）
	if err := setSetting(a.db, settingPushPlusToken, "topsecret"); err != nil {
		t.Fatalf("setSetting: %v", err)
	}
	if err := setSetting(a.db, settingServerChanSendKey, "SCTtopsecret"); err != nil {
		t.Fatalf("setSetting: %v", err)
	}
	rr = getJSON(t, h, "/api/settings")
	requireStatus(t, rr, http.StatusOK)
	var pub map[string]map[string]string
	json.NewDecoder(rr.Body).Decode(&pub)
	for k := range pub["data"] {
		if len(k) > 7 && k[:7] == "notify_" {
			t.Fatalf("public settings leaked secret key %q", k)
		}
	}
}

func TestMaskToken(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"abc", "****"},
		{"12345678", "****"},
		{"123456789", "12****6789"},
		{"abcdefghijklmnop", "ab****mnop"},
	}
	for _, tc := range cases {
		if got := maskToken(tc.in); got != tc.want {
			t.Fatalf("maskToken(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

// ======================================================================
// PushPlus 渠道发送（本地假服务器）
// ======================================================================

// fakePushPlus 启动一个模拟 pushplus API 的本地服务，返回其 URL。
func fakePushPlus(t *testing.T, code int) (*httptest.Server, *map[string]any) {
	t.Helper()
	payload := map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &payload)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":` + strconv.Itoa(code) + `,"msg":"mock","data":"pid"}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &payload
}

func overridePushPlusURL(t *testing.T, url string) {
	t.Helper()
	old := pushPlusSendURL
	pushPlusSendURL = url
	t.Cleanup(func() { pushPlusSendURL = old })
}

func TestPushPlusChannelSend(t *testing.T) {
	a := newTestApp(t)
	if err := setSetting(a.db, settingPushPlusEnabled, "1"); err != nil {
		t.Fatal(err)
	}
	if err := setSetting(a.db, settingPushPlusToken, "tok123"); err != nil {
		t.Fatal(err)
	}

	srv, payload := fakePushPlus(t, 200)
	overridePushPlusURL(t, srv.URL)

	ch := &pushPlusChannel{hc: srv.Client()}
	if ok, err := ch.configured(a.db); err != nil || !ok {
		t.Fatalf("configured=%v err=%v", ok, err)
	}
	msg := &NotifyMessage{Title: "标题", Content: "内容", Template: "markdown"}
	if err := ch.send(a.db, msg); err != nil {
		t.Fatalf("send: %v", err)
	}
	if (*payload)["token"] != "tok123" || (*payload)["title"] != "标题" ||
		(*payload)["content"] != "内容" || (*payload)["template"] != "markdown" {
		t.Fatalf("payload unexpected: %+v", *payload)
	}
	if _, has := (*payload)["topic"]; has {
		t.Fatal("topic should be omitted when empty")
	}

	// 业务错误码 → 失败
	errSrv, _ := fakePushPlus(t, 930)
	overridePushPlusURL(t, errSrv.URL)
	if err := ch.send(a.db, msg); err == nil {
		t.Fatal("expected error for business code != 200")
	}

	// 未启用 → configured=false
	_ = setSetting(a.db, settingPushPlusEnabled, "0")
	if ok, err := ch.configured(a.db); err != nil || ok {
		t.Fatalf("disabled should not be configured: ok=%v err=%v", ok, err)
	}
}

func TestNotifyTestEndpoint(t *testing.T) {
	a := newTestApp(t)
	a.auth = newAuthStore()
	h := a.authMiddleware(a.routes())
	c := adminCookie(t, a, h)

	// 未配置 → 400
	rr := reqWithCookie(t, h, http.MethodPost, "/api/notify/test", nil, c)
	requireStatus(t, rr, http.StatusBadRequest)

	// 配置后走本地假服务 → 成功结果
	srv, _ := fakePushPlus(t, 200)
	overridePushPlusURL(t, srv.URL)
	rr = reqWithCookie(t, h, http.MethodPut, "/api/notify/config",
		map[string]any{"pushplus": map[string]any{"enabled": 1, "token": "tok123"}}, c)
	requireStatus(t, rr, http.StatusOK)

	rr = reqWithCookie(t, h, http.MethodPost, "/api/notify/test", nil, c)
	requireStatus(t, rr, http.StatusOK)
	var resp struct {
		Data struct {
			Results []NotifyResult `json:"results"`
		} `json:"data"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp.Data.Results) != 1 {
		t.Fatalf("results: %+v", resp.Data.Results)
	}
	r0 := resp.Data.Results[0]
	if r0.Channel != "pushplus" || !r0.OK {
		t.Fatalf("result unexpected: %+v", r0)
	}
}
