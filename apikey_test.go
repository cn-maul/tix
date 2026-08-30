package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func apikeyDo(t *testing.T, h http.Handler, method, target string, headers map[string]string, cookie *http.Cookie, body any) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != nil {
		data, _ := json.Marshal(body)
		req = httptest.NewRequest(method, target, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func apikeyAdminCookie(t *testing.T, h http.Handler) *http.Cookie {
	t.Helper()
	rr := apikeyDo(t, h, http.MethodPost, "/api/login", nil, nil,
		map[string]string{"username": "testuser", "password": "secret"})
	if rr.Code != http.StatusOK {
		t.Fatalf("管理员登录失败: %d %s", rr.Code, rr.Body.String())
	}
	for _, c := range rr.Result().Cookies() {
		if c.Name == "tix_session" {
			return c
		}
	}
	t.Fatal("登录响应未下发会话 Cookie")
	return nil
}

func TestAPIKeyAuth(t *testing.T) {
	a := newTestApp(t)
	// 与 main.go 相同的中间件链（authMiddleware 内部走 requireAuth，覆盖 X-API-Key 逻辑）
	h := a.authMiddleware(a.routes())
	if _, err := createUser(a.db, "testuser", "secret", "测试用户", "admin"); err != nil {
		t.Fatalf("createUser: %v", err)
	}
	cookie := apikeyAdminCookie(t, h)

	// 未生成前 Key 为空
	rr := apikeyDo(t, h, http.MethodGet, "/api/settings/api-key", nil, cookie, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("查看 Key 失败: %d", rr.Code)
	}
	var got struct {
		Data struct {
			APIKey string `json:"api_key"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if got.Data.APIKey != "" {
		t.Fatalf("初始 Key 应为空，实际 %q", got.Data.APIKey)
	}

	// 非管理员不可查看/生成
	rr = apikeyDo(t, h, http.MethodGet, "/api/settings/api-key", nil, nil, nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("无会话查看 Key 应 401，实际 %d", rr.Code)
	}

	// 生成 Key
	rr = apikeyDo(t, h, http.MethodPost, "/api/settings/api-key/generate", nil, cookie, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("生成 Key 失败: %d %s", rr.Code, rr.Body.String())
	}
	var gen struct {
		Data struct {
			APIKey string `json:"api_key"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &gen)
	if len(gen.Data.APIKey) != 48 {
		t.Fatalf("生成的 Key 长度应为 48，实际 %q", gen.Data.APIKey)
	}

	// 持 Key 访问需登录接口
	rr = apikeyDo(t, h, http.MethodGet, "/api/tickets?status=0", map[string]string{"X-API-Key": gen.Data.APIKey}, nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("持有效 Key 访问工单列表应 200，实际 %d", rr.Code)
	}
	// 错误 Key → 401；无 Key → 401
	rr = apikeyDo(t, h, http.MethodGet, "/api/tickets?status=0", map[string]string{"X-API-Key": "bad-key"}, nil, nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("错误 Key 应 401，实际 %d", rr.Code)
	}
	rr = apikeyDo(t, h, http.MethodGet, "/api/tickets?status=0", nil, nil, nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("无凭据应 401，实际 %d", rr.Code)
	}
	// Key 不等于管理员：访问管理员接口应 403
	rr = apikeyDo(t, h, http.MethodGet, "/api/notify/config", map[string]string{"X-API-Key": gen.Data.APIKey}, nil, nil)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("持 Key 访问管理员接口应 403，实际 %d", rr.Code)
	}

	// 轮换后旧 Key 立即失效
	rr = apikeyDo(t, h, http.MethodPost, "/api/settings/api-key/generate", nil, cookie, nil)
	var gen2 struct {
		Data struct {
			APIKey string `json:"api_key"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &gen2)
	if gen2.Data.APIKey == gen.Data.APIKey {
		t.Fatal("轮换后的 Key 不应与旧 Key 相同")
	}
	rr = apikeyDo(t, h, http.MethodGet, "/api/tickets?status=0", map[string]string{"X-API-Key": gen.Data.APIKey}, nil, nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("旧 Key 应失效（401），实际 %d", rr.Code)
	}
	rr = apikeyDo(t, h, http.MethodGet, "/api/tickets?status=0", map[string]string{"X-API-Key": gen2.Data.APIKey}, nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("新 Key 应可用（200），实际 %d", rr.Code)
	}

	// 公开设置接口不得下发 api_key
	rr = apikeyDo(t, h, http.MethodGet, "/api/settings", nil, nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("公开设置接口失败: %d", rr.Code)
	}
	var pub map[string]map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &pub)
	if _, exists := pub["data"]["api_key"]; exists {
		t.Fatal("公开设置接口不应下发 api_key")
	}
}
