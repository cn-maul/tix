package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
)

func newTestApp(t *testing.T) *app {
	t.Helper()
	db, err := openDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := initDB(db); err != nil {
		t.Fatalf("initDB: %v", err)
	}
	if err := migrateDB(db); err != nil {
		t.Fatalf("migrateDB: %v", err)
	}
	return &app{db: db, notify: newNotifier()}
}

func postJSON(t *testing.T, h http.Handler, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var b *bytes.Buffer
	if body != nil {
		data, _ := json.Marshal(body)
		b = bytes.NewBuffer(data)
	} else {
		b = bytes.NewBuffer(nil)
	}
	req := httptest.NewRequest(http.MethodPost, target, b)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func getJSON(t *testing.T, h http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func putJSON(t *testing.T, h http.Handler, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, target, bytes.NewBuffer(data))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func requireStatus(t *testing.T, rr *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rr.Code != want {
		t.Fatalf("status=%d want %d, body=%s", rr.Code, want, rr.Body.String())
	}
}

// ---- 响应类型 ----

type ticketData struct{ Data Ticket `json:"data"` }
type listResp struct {
	Items []Ticket `json:"items"`
	Total int      `json:"total"`
	Page  int      `json:"page"`
	Size  int      `json:"size"`
}
type commentResp struct{ Items []Comment `json:"items"` }
type commentData struct{ Data Comment `json:"data"` }
type categoryResp struct{ Items []Category `json:"items"` }
type categoryData struct{ Data Category `json:"data"` }
type okResp struct{ OK bool `json:"ok"` }
type statsResp struct{ Data Stats `json:"data"` }
type healthResp struct{ OK bool `json:"ok"` }

// ======================================================================
// 健康检查
// ======================================================================

func TestHealth(t *testing.T) {
	a := newTestApp(t)
	rr := getJSON(t, a.routes(), "/api/health")
	requireStatus(t, rr, http.StatusOK)
	var resp healthResp
	json.NewDecoder(rr.Body).Decode(&resp)
	if !resp.OK {
		t.Fatal("health ok=false")
	}
}

// ======================================================================
// 分类种子与 CRUD
// ======================================================================

func TestCategorySeed(t *testing.T) {
	a := newTestApp(t)
	h := a.routes()
	var cr categoryResp
	rr := getJSON(t, h, "/api/categories")
	requireStatus(t, rr, http.StatusOK)
	json.NewDecoder(rr.Body).Decode(&cr)
	if len(cr.Items) != 5 {
		t.Fatalf("seed categories: got %d want 5", len(cr.Items))
	}
}

func TestCategoryCRUD(t *testing.T) {
	a := newTestApp(t)
	h := a.routes()

	// 创建
	rr := postJSON(t, h, "/api/categories", map[string]any{"name": "新增分类", "color": "#ef4444", "sort": 10})
	requireStatus(t, rr, http.StatusCreated)
	var cd categoryData
	json.NewDecoder(rr.Body).Decode(&cd)
	newID := cd.Data.ID

	// 更新
	rr = putJSON(t, h, "/api/categories/"+intToString(newID), map[string]any{"name": "修改分类", "color": "#10b981", "sort": 11, "enabled": 0})
	requireStatus(t, rr, http.StatusOK)
	json.NewDecoder(rr.Body).Decode(&cd)
	if cd.Data.Name != "修改分类" || cd.Data.Enabled != 0 {
		t.Fatalf("update category unexpected: %+v", cd.Data)
	}

	// 删除
	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/categories/"+intToString(newID), nil)
	h.ServeHTTP(rr, req)
	requireStatus(t, rr, http.StatusOK)

	// 校验：非法分类新建工单失败
	rr = postJSON(t, h, "/api/tickets", map[string]string{"category": "非法分类", "content": "c", "creator": "a"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid category: code=%d", rr.Code)
	}
}

func intToString(n int64) string {
	return strconv.FormatInt(n, 10)
}

// ======================================================================
// 工单完整流转
// ======================================================================

func TestTicketFlow(t *testing.T) {
	a := newTestApp(t)
	h := a.routes()

	// 创建
	var created ticketData
	rr := postJSON(t, h, "/api/tickets", map[string]any{
		"category": "软件问题", "content": "电脑蓝屏", "creator": "张三",
	})
	requireStatus(t, rr, http.StatusCreated)
	json.NewDecoder(rr.Body).Decode(&created)

	// 校验失败：内容为空
	rr = postJSON(t, h, "/api/tickets", map[string]string{"category": "软件问题", "content": " ", "creator": "张三"})
	requireStatus(t, rr, http.StatusBadRequest)

	// 列表（待处理）
	var list listResp
	rr = getJSON(t, h, "/api/tickets?status=0")
	requireStatus(t, rr, http.StatusOK)
	json.NewDecoder(rr.Body).Decode(&list)
	if len(list.Items) != 1 || list.Items[0].Content != "电脑蓝屏" {
		t.Fatalf("list unexpected: %+v", list.Items)
	}

	// 统计
	var stats statsResp
	rr = getJSON(t, h, "/api/stats")
	requireStatus(t, rr, http.StatusOK)
	json.NewDecoder(rr.Body).Decode(&stats)
	if stats.Data.Pending != 1 {
		t.Fatalf("pending=%d want 1", stats.Data.Pending)
	}

	// 详情
	var det ticketData
	rr = getJSON(t, h, "/api/tickets/1")
	requireStatus(t, rr, http.StatusOK)
	json.NewDecoder(rr.Body).Decode(&det)
	if det.Data.Content != "电脑蓝屏" {
		t.Fatal("detail content mismatch")
	}

	// 编辑
	rr = putJSON(t, h, "/api/tickets/1", map[string]any{"category": "网络问题", "content": "网络不通", "creator": "张三"})
	requireStatus(t, rr, http.StatusOK)
	rr = getJSON(t, h, "/api/tickets/1")
	json.NewDecoder(rr.Body).Decode(&det)
	if det.Data.Content != "网络不通" {
		t.Fatal("detail not reflecting edit")
	}

	// 追加备注
	rr = postJSON(t, h, "/api/tickets/1/comments", map[string]string{"author": "管理员", "content": "先排查网线"})
	requireStatus(t, rr, http.StatusCreated)
	var cdata commentData
	json.NewDecoder(rr.Body).Decode(&cdata)
	if cdata.Data.Author != "管理员" {
		t.Fatal("comment author mismatch")
	}

	// 标记已处理（带 note → 写备注）
	rr = postJSON(t, h, "/api/tickets/1/done", map[string]string{"note": "已修复交换机", "author": "工程师"})
	requireStatus(t, rr, http.StatusOK)
	rr = getJSON(t, h, "/api/tickets?status=0")
	json.NewDecoder(rr.Body).Decode(&list)
	if len(list.Items) != 0 {
		t.Fatal("pending list should be empty after done")
	}
	rr = getJSON(t, h, "/api/tickets?status=1")
	json.NewDecoder(rr.Body).Decode(&list)
	if len(list.Items) != 1 {
		t.Fatal("done list should have 1")
	}

	// 备注列表（两条：手动 + done 自动）
	var cre commentResp
	rr = getJSON(t, h, "/api/tickets/1/comments")
	requireStatus(t, rr, http.StatusOK)
	json.NewDecoder(rr.Body).Decode(&cre)
	if len(cre.Items) != 2 {
		t.Fatalf("comments count=%d want 2", len(cre.Items))
	}

	// 关键词搜索
	rr = getJSON(t, h, "/api/tickets?status=1&keyword=不通")
	requireStatus(t, rr, http.StatusOK)
	json.NewDecoder(rr.Body).Decode(&list)
	if len(list.Items) != 1 {
		t.Fatal("keyword search should hit")
	}

	// 删除
	rr = postJSON(t, h, "/api/tickets/1/delete", nil)
	requireStatus(t, rr, http.StatusOK)
	rr = getJSON(t, h, "/api/tickets/1")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("deleted ticket should be 404, got %d", rr.Code)
	}
}

// ======================================================================
// 分页 / 排序
// ======================================================================

func TestTicketPagination(t *testing.T) {
	a := newTestApp(t)
	for i := 0; i < 8; i++ {
		_, _ = createTicket(a.db, "其他", "工单"+strconv.Itoa(i), "张三")
	}
	h := a.routes()
	var list listResp
	rr := getJSON(t, h, "/api/tickets?status=0&page=1&size=3")
	requireStatus(t, rr, http.StatusOK)
	json.NewDecoder(rr.Body).Decode(&list)
	if len(list.Items) != 3 {
		t.Fatalf("page1 size: %d", len(list.Items))
	}
	if list.Total != 8 {
		t.Fatalf("total=%d want 8", list.Total)
	}
}

// ======================================================================
// 提交兼容别名 + 表单编码
// ======================================================================

func TestSubmitCompat(t *testing.T) {
	a := newTestApp(t)
	h := a.routes()
	rr := postJSON(t, h, "/api/submit", map[string]string{"category": "打印机故障", "content": "打印乱码", "creator": "李四"})
	requireStatus(t, rr, http.StatusCreated)

	// 表单编码
	req := httptest.NewRequest(http.MethodPost, "/api/submit",
		bytes.NewBufferString("category=软件问题&content=键盘坏&creator=王五"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	requireStatus(t, rr, http.StatusCreated)
}

// ======================================================================
// CSV 导出
// ======================================================================

func TestExportCSV(t *testing.T) {
	a := newTestApp(t)
	_, _ = createTicket(a.db, "软件问题", "测试导出", "张三")
	h := a.routes()
	rr := getJSON(t, h, "/api/export/csv?status=0")
	requireStatus(t, rr, http.StatusOK)
	ct := rr.Header().Get("Content-Type")
	if ct != "text/csv; charset=utf-8" {
		t.Fatalf("content-type=%s", ct)
	}
	body := rr.Body.String()
	if !bytes.Contains([]byte(body), []byte("测试导出")) {
		t.Fatalf("csv body missing content: %s", body)
	}
	// UTF-8 BOM
	if !bytes.HasPrefix(rr.Body.Bytes(), []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatalf("csv missing utf-8 bom")
	}
	// 公式注入防护
	_, _ = createTicket(a.db, "软件问题", "=HYPERLINK(\"http://evil\")", "张三")
	rr = getJSON(t, h, "/api/export/csv?status=0")
	requireStatus(t, rr, http.StatusOK)
	if !bytes.Contains(rr.Body.Bytes(), []byte("'=HYPERLINK")) {
		t.Fatalf("csv formula injection not neutralized")
	}
}

// ======================================================================
// SPA 回退纯净性（/submit 不含服务端渲染导航）
// ======================================================================

func TestSubmitBare(t *testing.T) {
	a := newTestApp(t)
	h := a.routes()
	rr := getJSON(t, h, "/submit")
	requireStatus(t, rr, http.StatusOK)
	body := rr.Body.String()
	for _, forbidden := range []string{"<a ", "待处理", "已处理", "工单列表"} {
		if bytes.Contains([]byte(body), []byte(forbidden)) {
			t.Fatalf("submit page should not contain %q", forbidden)
		}
	}
}

// ======================================================================
// 404
// ======================================================================

func Test404(t *testing.T) {
	a := newTestApp(t)
	h := a.routes()
	if rr := getJSON(t, h, "/api/tickets/999"); rr.Code != http.StatusNotFound {
		t.Fatalf("missing ticket: code=%d", rr.Code)
	}
	if rr := getJSON(t, h, "/api/tickets/abc"); rr.Code != http.StatusNotFound {
		t.Fatalf("invalid id: code=%d", rr.Code)
	}
}

// ======================================================================
// 迁移幂等性
// ======================================================================

func TestMigrateIdempotent(t *testing.T) {
	a := newTestApp(t)
	// 连续迁移两次不应报错
	if err := migrateDB(a.db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	// 分类种子不再重复写入
	var n int
	_ = a.db.QueryRow("SELECT COUNT(*) FROM categories").Scan(&n)
	if n != 5 {
		t.Fatalf("categories after re-migrate: %d want 5", n)
	}
	// 已删除 priority 列：模拟旧库再迁移一次也应成功
	if _, err := a.db.Exec("ALTER TABLE tickets ADD COLUMN priority INTEGER NOT NULL DEFAULT 2"); err != nil {
		t.Fatalf("re-add priority col: %v", err)
	}
	if err := migrateDB(a.db); err != nil {
		t.Fatalf("migrate with legacy priority col: %v", err)
	}
	has, _ := hasColumn(a.db, "tickets", "priority")
	if has {
		t.Fatal("priority column should be dropped")
	}
}

// ======================================================================
// 缺失资源操作 → 404
// ======================================================================

func TestMissingTicketOperations(t *testing.T) {
	a := newTestApp(t)
	h := a.routes()
	rr := postJSON(t, h, "/api/tickets/999/done", map[string]string{"note": "x"})
	requireStatus(t, rr, http.StatusNotFound)
	rr = postJSON(t, h, "/api/tickets/999/delete", nil)
	requireStatus(t, rr, http.StatusNotFound)
	rr = putJSON(t, h, "/api/categories/999", map[string]any{"name": "x"})
	requireStatus(t, rr, http.StatusNotFound)
	req := httptest.NewRequest(http.MethodDelete, "/api/categories/999", nil)
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req)
	requireStatus(t, rr2, http.StatusNotFound)
}

// ======================================================================
// 未注册 /api 路径 → 404 JSON
// ======================================================================

func TestUnknownAPI404(t *testing.T) {
	a := newTestApp(t)
	h := a.routes()
	for _, path := range []string{"/api/nonexistent", "/api/", "/api/tickets/1/nope"} {
		rr := getJSON(t, h, path)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("%s: code=%d want 404", path, rr.Code)
		}
	}
}

// ======================================================================
// order 参数校验
// ======================================================================

func TestOrderValidation(t *testing.T) {
	a := newTestApp(t)
	h := a.routes()
	if rr := getJSON(t, h, "/api/tickets?order=sideways"); rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid order: code=%d", rr.Code)
	}
	if rr := getJSON(t, h, "/api/tickets?order=ASC"); rr.Code != http.StatusOK {
		t.Fatalf("uppercase asc: code=%d", rr.Code)
	}
}

// ======================================================================
// 分类部分更新（不覆盖未提供字段）+ 停用分类不可建单
// ======================================================================

func TestCategoryPartialUpdate(t *testing.T) {
	a := newTestApp(t)
	h := a.routes()
	var cd categoryData
	rr := postJSON(t, h, "/api/categories", map[string]any{"name": "视频会议", "color": "#ef4444", "sort": 10})
	requireStatus(t, rr, http.StatusCreated)
	json.NewDecoder(rr.Body).Decode(&cd)
	id := cd.Data.ID

	// 只改 enabled，color/sort 应保留
	rr = putJSON(t, h, "/api/categories/"+intToString(id), map[string]any{"enabled": 0})
	requireStatus(t, rr, http.StatusOK)
	json.NewDecoder(rr.Body).Decode(&cd)
	if cd.Data.Color != "#ef4444" || cd.Data.Sort != 10 || cd.Data.Enabled != 0 {
		t.Fatalf("partial update clobbered fields: %+v", cd.Data)
	}

	// 停用后不可用于新建工单
	rr = postJSON(t, h, "/api/tickets", map[string]string{"category": "视频会议", "content": "c", "creator": "a"})
	requireStatus(t, rr, http.StatusBadRequest)

	// 重名分类 → 400
	rr = postJSON(t, h, "/api/categories", map[string]any{"name": "视频会议"})
	requireStatus(t, rr, http.StatusBadRequest)
}

// ======================================================================
// 提交页分类（公开）
// ======================================================================

func TestSubmitCategoriesPublic(t *testing.T) {
	a := newTestApp(t)
	h := a.routes()
	rr := getJSON(t, h, "/api/submit/categories")
	requireStatus(t, rr, http.StatusOK)
	var resp struct{ Items []string }
	json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp.Items) != 5 {
		t.Fatalf("expected 5 enabled categories, got %d", len(resp.Items))
	}
}

// ======================================================================
// 认证中间件
// ======================================================================

func TestAuthMiddleware(t *testing.T) {
	a := newTestApp(t)
	a.auth = newAuthStore()
	h := a.authMiddleware(a.routes())

	// 创建测试用户
	if _, err := createUser(a.db, "testuser", "secret", "测试用户", "admin"); err != nil {
		t.Fatalf("createUser: %v", err)
	}

	// 未登录访问受保护接口 → 401
	if rr := getJSON(t, h, "/api/stats"); rr.Code != http.StatusUnauthorized {
		t.Fatalf("unauthed stats: code=%d", rr.Code)
	}
	// 未登录访问公开接口 → 200
	if rr := getJSON(t, h, "/api/health"); rr.Code != http.StatusOK {
		t.Fatalf("unauthed health: code=%d", rr.Code)
	}
	if rr := getJSON(t, h, "/api/submit/categories"); rr.Code != http.StatusOK {
		t.Fatalf("unauthed submit categories: code=%d", rr.Code)
	}
	// 错误密码 → 401
	if rr := postJSON(t, h, "/api/login", map[string]string{"username": "testuser", "password": "wrong"}); rr.Code != http.StatusUnauthorized {
		t.Fatal("wrong password should 401")
	}
	// 正确密码 → 200 + cookie
	rr := postJSON(t, h, "/api/login", map[string]string{"username": "testuser", "password": "secret"})
	requireStatus(t, rr, http.StatusOK)
	var sess *http.Cookie
	for _, c := range rr.Result().Cookies() {
		if c.Name == "tix_session" {
			sess = c
			break
		}
	}
	if sess == nil {
		t.Fatal("no session cookie set")
	}

	// 带会话访问受保护接口 → 200
	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	req.AddCookie(sess)
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req)
	requireStatus(t, rr2, http.StatusOK)

	// 登出后会话失效
	req3 := httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	req3.AddCookie(sess)
	rr3 := httptest.NewRecorder()
	h.ServeHTTP(rr3, req3)
	requireStatus(t, rr3, http.StatusOK)
	req4 := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	req4.AddCookie(sess)
	rr4 := httptest.NewRecorder()
	h.ServeHTTP(rr4, req4)
	requireStatus(t, rr4, http.StatusUnauthorized)
}
