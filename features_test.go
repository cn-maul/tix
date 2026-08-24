package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

// ======================================================================
// 自助改密：PUT /api/profile/password
// ======================================================================

func TestProfilePasswordChange(t *testing.T) {
	a := newTestApp(t)
	if _, err := createUser(a.db, "selfpw", "oldpass66", "自助用户", "operator"); err != nil {
		t.Fatalf("createUser: %v", err)
	}
	h := a.authMiddleware(a.routes())
	c := loginAs(t, h, "selfpw", "oldpass66")

	// 旧密码错误 → 400
	rr := reqWithCookie(t, h, http.MethodPut, "/api/profile/password",
		map[string]string{"old_password": "wrong!", "new_password": "newpass66"}, c)
	requireStatus(t, rr, http.StatusBadRequest)

	// 新密码过短 → 400
	rr = reqWithCookie(t, h, http.MethodPut, "/api/profile/password",
		map[string]string{"old_password": "oldpass66", "new_password": "12345"}, c)
	requireStatus(t, rr, http.StatusBadRequest)

	// 正确修改 → 200，当前会话仍有效
	rr = reqWithCookie(t, h, http.MethodPut, "/api/profile/password",
		map[string]string{"old_password": "oldpass66", "new_password": "newpass66"}, c)
	requireStatus(t, rr, http.StatusOK)
	rr = reqWithCookie(t, h, http.MethodGet, "/api/auth/status", nil, c)
	requireStatus(t, rr, http.StatusOK)
	var st struct {
		Data struct {
			OK bool `json:"ok"`
		} `json:"data"`
	}
	json.NewDecoder(rr.Body).Decode(&st)
	if !st.Data.OK {
		t.Fatal("current session should survive self password change")
	}

	// 其他端会话已被吊销：用旧会话 cookie 副本模拟另一端
	cOther := &http.Cookie{Name: c.Name, Value: c.Value[:32] + "ffff", Path: "/"}
	_ = cOther // 会话令牌随机无法复制，改为验证新密码可登录、旧密码失效

	rr = postJSON(t, h, "/api/login", map[string]string{"username": "selfpw", "password": "oldpass66"})
	requireStatus(t, rr, http.StatusUnauthorized)
	c2 := loginAs(t, h, "selfpw", "newpass66")
	if c2 == nil {
		t.Fatal("login with new password failed")
	}
}

func TestProfilePasswordRevokesOtherSessions(t *testing.T) {
	a := newTestApp(t)
	a.loginLimiter = newRateLimiter(1000, time.Minute)
	if _, err := createUser(a.db, "multi", "oldpass66", "多端用户", "operator"); err != nil {
		t.Fatalf("createUser: %v", err)
	}
	h := a.authMiddleware(a.routes())

	// 同一用户登录两个会话
	cA := loginAs(t, h, "multi", "oldpass66")
	cB := loginAs(t, h, "multi", "oldpass66")

	// A 端自助改密 → B 端会话应失效，A 端保持
	rr := reqWithCookie(t, h, http.MethodPut, "/api/profile/password",
		map[string]string{"old_password": "oldpass66", "new_password": "newpass66"}, cA)
	requireStatus(t, rr, http.StatusOK)

	rr = reqWithCookie(t, h, http.MethodGet, "/api/auth/status", nil, cA)
	requireStatus(t, rr, http.StatusOK)
	var st struct {
		Data struct {
			OK bool `json:"ok"`
		} `json:"data"`
	}
	json.NewDecoder(rr.Body).Decode(&st)
	if !st.Data.OK {
		t.Fatal("session A should stay logged in")
	}
	rr = reqWithCookie(t, h, http.MethodGet, "/api/stats", nil, cB)
	requireStatus(t, rr, http.StatusUnauthorized)
}

// ======================================================================
// 时间范围筛选：from/to（含边界日）
// ======================================================================

func seedTicketAt(t *testing.T, a *app, day string) int64 {
	t.Helper()
	id, err := createTicket(a.db, "软件问题", "内容-"+day, "张三", "13800138000")
	if err != nil {
		t.Fatalf("createTicket: %v", err)
	}
	if _, err := a.db.Exec("UPDATE tickets SET created_at = ? WHERE id = ?", day+" 10:30:00", id); err != nil {
		t.Fatalf("set created_at: %v", err)
	}
	return id
}

func listCount(t *testing.T, h http.Handler, query string) int {
	t.Helper()
	rr := getJSON(t, h, "/api/tickets?"+query)
	requireStatus(t, rr, http.StatusOK)
	var resp listResp
	json.NewDecoder(rr.Body).Decode(&resp)
	return len(resp.Items)
}

func listCountCookie(t *testing.T, h http.Handler, query string, c *http.Cookie) int {
	t.Helper()
	rr := reqWithCookie(t, h, http.MethodGet, "/api/tickets?"+query, nil, c)
	requireStatus(t, rr, http.StatusOK)
	var resp listResp
	json.NewDecoder(rr.Body).Decode(&resp)
	return len(resp.Items)
}

func TestTicketDateRangeFilter(t *testing.T) {
	a := newTestApp(t)
	h := a.routes()
	seedTicketAt(t, a, "2026-01-05")
	seedTicketAt(t, a, "2026-01-20")
	seedTicketAt(t, a, "2026-02-10")

	if got := listCount(t, h, ""); got != 3 {
		t.Fatalf("no filter: %d want 3", got)
	}
	if got := listCount(t, h, "from=2026-01-01&to=2026-01-31"); got != 2 {
		t.Fatalf("january: %d want 2", got)
	}
	// 边界日包含：当天 00:00:00 与 23:59:59
	if got := listCount(t, h, "from=2026-01-05&to=2026-01-05"); got != 1 {
		t.Fatalf("boundary day: %d want 1", got)
	}
	if got := listCount(t, h, "to=2026-01-31"); got != 2 {
		t.Fatalf("until january: %d want 2", got)
	}
	if got := listCount(t, h, "from=2026-02-01"); got != 1 {
		t.Fatalf("since february: %d want 1", got)
	}

	// 非法日期格式 → 400
	if rr := getJSON(t, h, "/api/tickets?from=20260101"); rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid from: %d", rr.Code)
	}
	if rr := getJSON(t, h, "/api/tickets?to=2026-13-01"); rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid to: %d", rr.Code)
	}
}

// ======================================================================
// 指派负责人：assign 接口 + 列表筛选
// ======================================================================

func TestTicketAssign(t *testing.T) {
	a := newTestApp(t)
	h := a.routes()
	if _, err := createUser(a.db, "worker", "secret123", "干活的", "operator"); err != nil {
		t.Fatalf("createUser: %v", err)
	}
	var created ticketData
	rr := postJSON(t, h, "/api/tickets", map[string]any{"category": "软件问题", "content": "待指派", "name": "张三", "phone": "13800138000"})
	requireStatus(t, rr, http.StatusCreated)
	json.NewDecoder(rr.Body).Decode(&created)

	// 指派给不存在用户 → 400
	rr = postJSON(t, h, "/api/tickets/"+intToString(created.Data.ID)+"/assign",
		map[string]string{"assignee": "ghost"})
	requireStatus(t, rr, http.StatusBadRequest)

	// 指派成功：字段更新 + 写备注流水
	rr = postJSON(t, h, "/api/tickets/"+intToString(created.Data.ID)+"/assign",
		map[string]string{"assignee": "worker"})
	requireStatus(t, rr, http.StatusOK)
	json.NewDecoder(rr.Body).Decode(&created)
	if created.Data.Assignee != "worker" {
		t.Fatalf("assignee=%q want worker", created.Data.Assignee)
	}
	var cre commentResp
	rr = getJSON(t, h, "/api/tickets/"+intToString(created.Data.ID)+"/comments")
	json.NewDecoder(rr.Body).Decode(&cre)
	if len(cre.Items) != 1 || !strings.Contains(cre.Items[0].Content, "【指派】") {
		t.Fatalf("assign comment missing: %+v", cre.Items)
	}

	// 取消指派
	rr = postJSON(t, h, "/api/tickets/"+intToString(created.Data.ID)+"/assign", map[string]string{"assignee": ""})
	requireStatus(t, rr, http.StatusOK)
	json.NewDecoder(rr.Body).Decode(&created)
	if created.Data.Assignee != "" {
		t.Fatalf("assignee should be cleared, got %q", created.Data.Assignee)
	}
}

func TestTicketAssigneeFilter(t *testing.T) {
	a := newTestApp(t)
	a.loginLimiter = newRateLimiter(1000, time.Minute)
	if _, err := createUser(a.db, "alice", "secret123", "甲", "operator"); err != nil {
		t.Fatalf("createUser: %v", err)
	}
	h := a.authMiddleware(a.routes())
	c := loginAs(t, h, "alice", "secret123")

	for i := 0; i < 3; i++ {
		rr := reqWithCookie(t, h, http.MethodPost, "/api/tickets",
			map[string]any{"category": "软件问题", "content": "x", "name": "张三", "phone": "13800138000"}, c)
		requireStatus(t, rr, http.StatusCreated)
	}
	rr := reqWithCookie(t, h, http.MethodPost, "/api/tickets/1/assign",
		map[string]string{"assignee": "alice"}, c)
	requireStatus(t, rr, http.StatusOK)

	// 未登录访问 assignee=me → 401（me 需要解析会话）
	if rr := getJSON(t, h, "/api/tickets?assignee=me"); rr.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous me: %d", rr.Code)
	}
	// 我负责的
	if got := listCountCookie(t, h, "assignee=me", c); got != 1 {
		t.Fatalf("assignee=me: %d want 1", got)
	}
	// 未指派
	if got := listCountCookie(t, h, "unassigned=1", c); got != 2 {
		t.Fatalf("unassigned: %d want 2", got)
	}
	_ = c
}

// ======================================================================
// 批量操作：batch-done / batch-delete
// ======================================================================

func TestTicketBatchDoneAndDelete(t *testing.T) {
	a := newTestApp(t)
	h := a.routes()
	for i := 0; i < 4; i++ {
		postJSON(t, h, "/api/tickets", map[string]any{"category": "软件问题", "content": "批量" + strconv.Itoa(i), "name": "张三", "phone": "13800138000"})
	}
	// 给工单 1 加备注，验证批量删除连带清理
	rr := postJSON(t, h, "/api/tickets/1/comments", map[string]string{"author": "管理员", "content": "备注"})
	requireStatus(t, rr, http.StatusCreated)

	// 空 ids → 400
	rr = postJSON(t, h, "/api/tickets/batch-done", map[string]any{"ids": []int64{}})
	requireStatus(t, rr, http.StatusBadRequest)
	// 非法值 → 400
	rr = postJSON(t, h, "/api/tickets/batch-done", map[string]any{"ids": []int64{1, -2}})
	requireStatus(t, rr, http.StatusBadRequest)
	// 超上限 → 400
	big := make([]int64, maxBatchIDs+1)
	for i := range big {
		big[i] = int64(i + 1)
	}
	rr = postJSON(t, h, "/api/tickets/batch-done", map[string]any{"ids": big})
	requireStatus(t, rr, http.StatusBadRequest)

	// 批量已处理（含不存在的 id 9，容错跳过）
	rr = postJSON(t, h, "/api/tickets/batch-done", map[string]any{"ids": []int64{1, 2, 999}})
	requireStatus(t, rr, http.StatusOK)
	var br struct {
		Data struct {
			Updated int64 `json:"updated"`
		} `json:"data"`
	}
	json.NewDecoder(rr.Body).Decode(&br)
	if br.Data.Updated != 2 {
		t.Fatalf("updated=%d want 2", br.Data.Updated)
	}
	if got := listCount(t, h, "status=1"); got != 2 {
		t.Fatalf("done count: %d want 2", got)
	}

	// 批量删除（含备注连带清理）
	rr = postJSON(t, h, "/api/tickets/batch-delete", map[string]any{"ids": []int64{1, 3}})
	requireStatus(t, rr, http.StatusOK)
	if got := listCount(t, h, ""); got != 2 {
		t.Fatalf("after delete: %d want 2", got)
	}
	var n int
	_ = a.db.QueryRow("SELECT COUNT(*) FROM comments WHERE ticket_id = 1").Scan(&n)
	if n != 0 {
		t.Fatalf("comments of deleted ticket should be gone, got %d", n)
	}
}

// ======================================================================
// Server酱 渠道发送（本地假服务）
// ======================================================================

func overrideServerChanBase(t *testing.T, base string) {
	t.Helper()
	old := serverChanAPIBase
	serverChanAPIBase = base
	t.Cleanup(func() { serverChanAPIBase = old })
}

func fakeServerChan(t *testing.T, code int) (*httptest.Server, *url.Values) {
	t.Helper()
	form := url.Values{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		for k, vs := range r.PostForm {
			form[k] = vs
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":` + strconv.Itoa(code) + `,"message":"mock","data":{"pushid":"x"}}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &form
}

func TestServerChanChannelSend(t *testing.T) {
	a := newTestApp(t)
	if err := setSetting(a.db, settingServerChanEnabled, "1"); err != nil {
		t.Fatal(err)
	}
	if err := setSetting(a.db, settingServerChanSendKey, "SCT000111222"); err != nil {
		t.Fatal(err)
	}

	srv, form := fakeServerChan(t, 0)
	overrideServerChanBase(t, srv.URL+"/")

	ch := &serverChanChannel{hc: srv.Client()}
	if ok, err := ch.configured(a.db); err != nil || !ok {
		t.Fatalf("configured=%v err=%v", ok, err)
	}
	msg := &NotifyMessage{Title: "标题", Content: "**正文**"}
	if err := ch.send(a.db, msg); err != nil {
		t.Fatalf("send: %v", err)
	}
	if form.Get("title") != "标题" || form.Get("desp") != "**正文**" {
		t.Fatalf("form unexpected: %+v", *form)
	}

	// 业务错误码 → 失败
	errSrv, _ := fakeServerChan(t, 40001)
	overrideServerChanBase(t, errSrv.URL+"/")
	if err := ch.send(a.db, msg); err == nil {
		t.Fatal("expected error for business code != 0")
	}

	// 未启用 → configured=false
	_ = setSetting(a.db, settingServerChanEnabled, "0")
	if ok, err := ch.configured(a.db); err != nil || ok {
		t.Fatalf("disabled should not be configured: ok=%v err=%v", ok, err)
	}
}

// ======================================================================
// 游客进度查询：/api/my/tickets（公开，凭手机号后缀匹配 creator）
// ======================================================================

func TestGuestTicketTracking(t *testing.T) {
	a := newTestApp(t)
	// 本用例请求数远超限流阈值，放宽以免误触 429（生产仍为 10 次/分钟）
	a.submitLimiter = newRateLimiter(1000, time.Minute)
	h := a.routes() // 游客接口不走鉴权中间件

	// 通过公开提交接口创建三单（张三/李四，姓名与手机号独立字段）
	postJSON(t, h, "/api/submit", map[string]any{"category": "软件问题", "content": "蓝屏", "name": "张三", "phone": "13800138000"})
	postJSON(t, h, "/api/submit", map[string]any{"category": "网络问题", "content": "断网", "name": "张三", "phone": "13800138000"})
	postJSON(t, h, "/api/submit", map[string]any{"category": "软件问题", "content": "别人的", "name": "李四", "phone": "13900139000"})
	// 直接入库一条无手机号的旧数据（绕过 API 校验，模拟历史遗留），不应被手机号查到
	if _, err := createTicket(a.db, "软件问题", "旧数据无名", "王老", ""); err != nil {
		t.Fatalf("createTicket legacy: %v", err)
	}

	// 管理员处理 1 号工单：指派 + 留言 + 标记已处理（产生系统备注和处理人留言）
	if _, err := createUser(a.db, "handler", "handler66", "处理员", "admin"); err != nil {
		t.Fatalf("createUser: %v", err)
	}
	hAuth := a.authMiddleware(a.routes())
	c := loginAs(t, hAuth, "handler", "handler66")
	if rr := reqWithCookie(t, hAuth, http.MethodPost, "/api/tickets/1/assign",
		map[string]string{"assignee": "handler"}, c); rr.Code != http.StatusOK {
		t.Fatalf("assign: %d", rr.Code)
	}
	if rr := reqWithCookie(t, hAuth, http.MethodPost, "/api/tickets/1/comments",
		map[string]string{"author": "管理员", "content": "已重装系统"}, c); rr.Code != http.StatusCreated {
		t.Fatalf("comment: %d", rr.Code)
	}
	if rr := reqWithCookie(t, hAuth, http.MethodPost, "/api/tickets/1/done",
		map[string]any{"note": "修复完成", "author": "管理员"}, c); rr.Code != http.StatusOK {
		t.Fatalf("done: %d", rr.Code)
	}

	// 列表：只含张三（该手机号）名下的工单，新的在前；无手机号的旧工单不出现
	rr := getJSON(t, h, "/api/my/tickets?phone=13800138000")
	requireStatus(t, rr, http.StatusOK)
	var list struct {
		Items []struct {
			ID     int    `json:"id"`
			Status int    `json:"status"`
			Name   string `json:"creator"`
		} `json:"items"`
		Total int `json:"total"`
	}
	json.NewDecoder(rr.Body).Decode(&list)
	if list.Total != 2 || len(list.Items) != 2 {
		t.Fatalf("total=%d items=%d want 2", list.Total, len(list.Items))
	}
	if list.Items[0].ID != 2 || list.Items[1].ID != 1 {
		t.Fatalf("order unexpected: %d,%d want 2,1", list.Items[0].ID, list.Items[1].ID)
	}
	if list.Items[1].Status != 1 {
		t.Fatalf("ticket 1 should be done, got %d", list.Items[1].Status)
	}

	// 详情：能看到状态 + 处理记录（含谁处理的、留言内容）
	rr = getJSON(t, h, "/api/my/tickets/1?phone=13800138000")
	requireStatus(t, rr, http.StatusOK)
	var detail struct {
		Data struct {
			Ticket struct {
				Status   int    `json:"status"`
				Assignee string `json:"assignee"`
			} `json:"ticket"`
			Comments []struct {
				Author  string `json:"author"`
				Content string `json:"content"`
			} `json:"comments"`
		} `json:"data"`
	}
	json.NewDecoder(rr.Body).Decode(&detail)
	if detail.Data.Ticket.Status != 1 || detail.Data.Ticket.Assignee != "handler" {
		t.Fatalf("detail unexpected: %+v", detail.Data.Ticket)
	}
	foundComment := false
	for _, cm := range detail.Data.Comments {
		if cm.Author == "管理员" && cm.Content == "已重装系统" {
			foundComment = true
		}
	}
	if !foundComment {
		t.Fatalf("handler comment not visible: %+v", detail.Data.Comments)
	}

	// 他人手机号查不到张三的工单
	rr = getJSON(t, h, "/api/my/tickets/1?phone=13900139000")
	requireStatus(t, rr, http.StatusNotFound)

	// 缺参数 / 非法参数（非 11 位、含非数字）→ 400
	requireStatus(t, getJSON(t, h, "/api/my/tickets"), http.StatusBadRequest)
	requireStatus(t, getJSON(t, h, "/api/my/tickets?phone=12345"), http.StatusBadRequest)
	requireStatus(t, getJSON(t, h, "/api/my/tickets?phone=1380013800a"), http.StatusBadRequest)
	// 11 位数字但非手机号段（第二位须为 3-9）→ 400（与创建校验同规则）
	requireStatus(t, getJSON(t, h, "/api/my/tickets?phone=12345678901"), http.StatusBadRequest)

	// 不存在的工单 → 404
	requireStatus(t, getJSON(t, h, "/api/my/tickets/999?phone=13800138000"), http.StatusNotFound)
}

// ======================================================================
// 迁移：migrateSplitCreatorPhone 把旧版拼接 creator 拆分为姓名 + phone 列
// ======================================================================

func TestMigrateSplitCreatorPhone(t *testing.T) {
	a := newTestApp(t)

	// 直接入库模拟旧版各种填写格式（phone 列为空 = 未迁移）
	legacyFormats := []string{
		"张三13800138000",  // 标准：姓名+手机号 → 应拆分
		"13800138001",    // 整串就是手机号 → 提取手机号，creator 原样保留
		"李四 13900139000", // 姓名与手机号间有空格 → 拆分并去除尾部空格
		"王五",             // 纯姓名无手机号 → 不动（无法查询属预期）
		"工号12345678901",  // 尾部 11 位但非手机号段（第二位是 2）→ 不动
		"张三1380013800",   // 尾部只有 10 位数字 → 不动
	}
	for i, c := range legacyFormats {
		if _, err := a.db.Exec(
			"INSERT INTO tickets (category, content, creator, phone, status, created_at, updated_at) "+
				"VALUES ('软件问题', ?, ?, '', 0, '2026-01-01 00:00:00', '2026-01-01 00:00:00')",
			fmt.Sprintf("内容%d", i+1), c); err != nil {
			t.Fatalf("seed row %d: %v", i+1, err)
		}
	}

	if err := migrateSplitCreatorPhone(a.db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	want := []struct{ creator, phone string }{
		{"张三", "13800138000"},
		{"13800138001", "13800138001"},
		{"李四", "13900139000"},
		{"王五", ""},
		{"工号12345678901", ""},
		{"张三1380013800", ""},
	}
	for i, w := range want {
		var gotC, gotP string
		if err := a.db.QueryRow("SELECT creator, phone FROM tickets WHERE id = ?", i+1).Scan(&gotC, &gotP); err != nil {
			t.Fatalf("row %d: %v", i+1, err)
		}
		if gotC != w.creator || gotP != w.phone {
			t.Errorf("row %d: got (%q, %q) want (%q, %q)", i+1, gotC, gotP, w.creator, w.phone)
		}
	}

	// 幂等：重复执行结果不变
	if err := migrateSplitCreatorPhone(a.db); err != nil {
		t.Fatalf("migrate again: %v", err)
	}
	for i, w := range want {
		var gotC, gotP string
		if err := a.db.QueryRow("SELECT creator, phone FROM tickets WHERE id = ?", i+1).Scan(&gotC, &gotP); err != nil {
			t.Fatalf("row %d (2nd pass): %v", i+1, err)
		}
		if gotC != w.creator || gotP != w.phone {
			t.Errorf("row %d (2nd pass): got (%q, %q) want (%q, %q)", i+1, gotC, gotP, w.creator, w.phone)
		}
	}
}

// ======================================================================
// 标记已处理始终留痕 + 迁移补写「已处理完成」
// ======================================================================

func TestDoneAlwaysWritesRecord(t *testing.T) {
	a := newTestApp(t)
	if _, err := createUser(a.db, "h", "handler66", "处理员", "admin"); err != nil {
		t.Fatalf("createUser: %v", err)
	}
	h := a.authMiddleware(a.routes())
	c := loginAs(t, h, "h", "handler66")

	// 建两单：一单带备注完成，一单不带备注完成
	for _, content := range []string{"带备注", "无备注"} {
		rr := reqWithCookie(t, h, http.MethodPost, "/api/tickets",
			map[string]any{"category": "软件问题", "content": content, "name": "张三", "phone": "13800138000"}, c)
		requireStatus(t, rr, http.StatusCreated)
	}
	rr := reqWithCookie(t, h, http.MethodPost, "/api/tickets/1/done",
		map[string]any{"note": "已修复", "author": "处理员"}, c)
	requireStatus(t, rr, http.StatusOK)
	rr = reqWithCookie(t, h, http.MethodPost, "/api/tickets/2/done", nil, c)
	requireStatus(t, rr, http.StatusOK)

	countContents := func(ticketID int64) []string {
		cc, err := listComments(a.db, ticketID)
		if err != nil {
			t.Fatalf("listComments: %v", err)
		}
		out := make([]string, 0, len(cc))
		for _, cm := range cc {
			out = append(out, cm.Content)
		}
		return out
	}

	// 带备注：记录含【标记已处理】+ 备注
	got := countContents(1)
	if len(got) != 1 || got[0] != "【标记已处理】已修复" {
		t.Fatalf("ticket1 comments: %v", got)
	}
	// 无备注：也应有一条【标记已处理】
	got = countContents(2)
	if len(got) != 1 || got[0] != "【标记已处理】" {
		t.Fatalf("ticket2 comments: %v", got)
	}

	// 批量已处理：为每条写入【批量标记已处理】，作者取请求体（缺省系统）
	postJSON(t, h, "/api/submit", map[string]any{"category": "网络问题", "content": "批量A", "name": "李四", "phone": "13900139000"})
	postJSON(t, h, "/api/submit", map[string]any{"category": "网络问题", "content": "批量B", "name": "李四", "phone": "13900139000"})
	rr = reqWithCookie(t, h, http.MethodPost, "/api/tickets/batch-done",
		map[string]any{"ids": []int{3, 4}, "author": "处理员"}, c)
	requireStatus(t, rr, http.StatusOK)
	for _, id := range []int64{3, 4} {
		got = countContents(id)
		if len(got) != 1 || got[0] != "【批量标记已处理】" {
			t.Fatalf("ticket%d batch comments: %v", id, got)
		}
	}
}

func TestMigrateBackfillDoneRecords(t *testing.T) {
	a := newTestApp(t)

	seed := func(content string, status int, withComment bool) int64 {
		id, err := createTicket(a.db, "软件问题", content, "张三", "13800138000")
		if err != nil {
			t.Fatalf("createTicket: %v", err)
		}
		if _, err := a.db.Exec("UPDATE tickets SET status = ? WHERE id = ?", status, id); err != nil {
			t.Fatalf("set status: %v", err)
		}
		if withComment {
			if _, _, err := addComment(a.db, id, "管理员", "已有记录"); err != nil {
				t.Fatalf("addComment: %v", err)
			}
		}
		return id
	}
	doneNoComment := seed("已完成无记录", 1, false)
	pendingNoComment := seed("待处理无记录", 0, false)
	doneWithComment := seed("已完成有记录", 1, true)

	if err := migrateBackfillDoneRecords(a.db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	commentCount := func(id int64) int {
		cc, err := listComments(a.db, id)
		if err != nil {
			t.Fatalf("listComments: %v", err)
		}
		return len(cc)
	}
	if n := commentCount(doneNoComment); n != 1 {
		t.Fatalf("done-no-comment records=%d want 1", n)
	}
	if cc, _ := listComments(a.db, doneNoComment); cc[0].Content != "【已处理完成】" || cc[0].Author != "系统" {
		t.Fatalf("backfill record unexpected: %+v", cc[0])
	}
	if n := commentCount(pendingNoComment); n != 0 {
		t.Fatalf("pending should have no records, got %d", n)
	}
	if n := commentCount(doneWithComment); n != 1 {
		t.Fatalf("done-with-comment should keep original only, got %d", n)
	}

	// 幂等：重复执行不再新增
	if err := migrateBackfillDoneRecords(a.db); err != nil {
		t.Fatalf("migrate again: %v", err)
	}
	if n := commentCount(doneNoComment); n != 1 {
		t.Fatalf("idempotent check failed, records=%d", n)
	}
}
