package main

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ---------- 通用响应 ----------

func jsonResp(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, code int, msg string) {
	jsonResp(w, code, map[string]any{
		"error": map[string]any{"code": code, "message": msg},
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	if r.Body == nil {
		return fmt.Errorf("请求体为空")
	}
	defer r.Body.Close()
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB 上限
	return json.NewDecoder(r.Body).Decode(v)
}

func parseID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

// parseDateParam 校验 YYYY-MM-DD 日期参数；空串合法（表示不筛选）。
func parseDateParam(v string) (string, bool) {
	if v == "" {
		return "", true
	}
	if _, err := time.ParseInLocation("2006-01-02", v, time.Local); err != nil {
		return "", false
	}
	return v, true
}

// buildTicketQuery 从查询参数构造工单列表条件。
// 非法参数（status/from/to/order）返回 ok=false。
func buildTicketQuery(q url.Values, page, size int) (ticketQuery, bool) {
	status := -1
	if v := q.Get("status"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || (n != 0 && n != 1) {
			return ticketQuery{}, false
		}
		status = n
	}
	from, ok := parseDateParam(q.Get("from"))
	if !ok {
		return ticketQuery{}, false
	}
	to, ok := parseDateParam(q.Get("to"))
	if !ok {
		return ticketQuery{}, false
	}
	order := q.Get("order")
	if order == "" {
		order = "desc"
	}
	if !strings.EqualFold(order, "asc") && !strings.EqualFold(order, "desc") {
		return ticketQuery{}, false
	}
	return ticketQuery{
		status:     status,
		category:   q.Get("category"),
		keyword:    strings.TrimSpace(q.Get("keyword")),
		from:       from,
		to:         to,
		assignee:   q.Get("assignee"),
		unassigned: q.Get("unassigned") == "1",
		page:       page,
		size:       size,
		order:      order,
	}, true
}

// --------------------------------------------------------------------
// 健康检查
// --------------------------------------------------------------------

func (a *app) apiHealth(w http.ResponseWriter, r *http.Request) {
	jsonResp(w, http.StatusOK, map[string]any{"ok": true})
}

// --------------------------------------------------------------------
// 统计
// --------------------------------------------------------------------

func (a *app) apiStats(w http.ResponseWriter, r *http.Request) {
	s, err := getStats(a.db)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	jsonResp(w, http.StatusOK, map[string]any{"data": s})
}

// --------------------------------------------------------------------
// 工单列表（分页 + 状态/分类/关键词/时间范围/负责人筛选）
// --------------------------------------------------------------------

func (a *app) apiTicketList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("size"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}

	tq, ok := buildTicketQuery(q, page, size)
	if !ok {
		jsonError(w, http.StatusBadRequest, "查询参数非法（status/from/to/order）")
		return
	}
	// assignee=me 由服务端按当前会话用户名解析
	if tq.assignee == "me" {
		sess := a.requireAuth(w, r)
		if sess == nil {
			return
		}
		tq.assignee = sess.username
	}

	items, total, err := listTickets(a.db, tq)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	jsonResp(w, http.StatusOK, map[string]any{
		"items": items, "total": total, "page": page, "size": size,
	})
}

// --------------------------------------------------------------------
// 新建工单
// --------------------------------------------------------------------

func (a *app) apiTicketCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Category string `json:"category"`
		Content  string `json:"content"`
		Name     string `json:"name"`    // 发起人姓名（推荐）
		Phone    string `json:"phone"`   // 发起人手机号（游客进度查询凭据）
		Creator  string `json:"creator"` // 旧版兼容：整串「姓名+手机号」，自动拆分
	}
	if err := decodeJSON(w, r, &body); err != nil {
		jsonError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	name, phone := body.Name, body.Phone
	if name == "" && phone == "" && body.Creator != "" {
		// 旧版外部表单仍传整串 creator：按尾部手机号拆分
		name, phone = splitCreatorPhone(strings.TrimSpace(body.Creator))
	}
	a.createTicketFromInput(w,
		strings.TrimSpace(body.Category),
		strings.TrimSpace(body.Content),
		strings.TrimSpace(name), strings.TrimSpace(phone))
}

// splitCreatorPhone 拆分旧版「姓名+手机号」拼接串；无尾部手机号时视为纯姓名。
func splitCreatorPhone(creator string) (name, phone string) {
	if m := legacyPhoneTailRe.FindStringSubmatch(creator); m != nil {
		return strings.TrimSpace(strings.TrimSuffix(creator, m[1])), m[1]
	}
	return creator, ""
}

// createTicketFromInput 校验并创建工单：JSON 与表单两条提交路径共用的落库与响应逻辑。
func (a *app) createTicketFromInput(w http.ResponseWriter, category, content, name, phone string) {
	cat, txt, who, tel, errMsg := validateTicketFields(a.db, category, content, name, phone)
	if errMsg != "" {
		jsonError(w, http.StatusBadRequest, errMsg)
		return
	}
	id, err := createTicket(a.db, cat, txt, who, tel)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "提交失败")
		return
	}
	invalidateStatsCache()
	t, _ := getTicket(a.db, id)
	jsonResp(w, http.StatusCreated, map[string]any{"data": t})
}

func (a *app) apiSubmitCompat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		return
	}

	// Rate limiting: 同IP每分钟最多10次
	ip := a.clientIP(r)
	if !a.submitLimiter.allow(ip) {
		jsonError(w, http.StatusTooManyRequests, "提交过于频繁，请稍后再试")
		return
	}

	ct := r.Header.Get("Content-Type")
	if ct == "" || strings.HasPrefix(ct, "application/json") {
		a.apiTicketCreate(w, r)
		return
	}
	// 表单分支同样限制请求体大小，与 JSON 分支保持一致
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	_ = r.ParseForm()
	name := strings.TrimSpace(r.FormValue("name"))
	phone := strings.TrimSpace(r.FormValue("phone"))
	if name == "" && phone == "" {
		// 旧版外部表单只传整串 creator：自动拆分
		name, phone = splitCreatorPhone(strings.TrimSpace(r.FormValue("creator")))
	}
	a.createTicketFromInput(w,
		r.FormValue("category"),
		strings.TrimSpace(r.FormValue("content")),
		name, phone)
}

// apiSubmitCategories 提交页公开接口：返回已启用的分类名（不含元数据）。
func (a *app) apiSubmitCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := enabledCategoryNames(a.db)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	jsonResp(w, http.StatusOK, map[string]any{"items": cats})
}

// --------------------------------------------------------------------
// 游客进度查询（公开；凭提交时填写的手机号查询）
// --------------------------------------------------------------------

// guestTrackPhone 校验游客查询参数：11 位大陆手机号（与创建时的 validPhone 同规则）。
func guestTrackPhone(r *http.Request) (string, bool) {
	phone := strings.TrimSpace(r.URL.Query().Get("phone"))
	if !validPhone(phone) {
		return "", false
	}
	return phone, true
}

// apiMyTickets GET /api/my/tickets?phone=…（公开）：游客查看自己提交的工单列表。
func (a *app) apiMyTickets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		return
	}
	// 与提交共用同一限流桶：防止遍历枚举他人工单
	if !a.submitLimiter.allow(a.clientIP(r)) {
		jsonError(w, http.StatusTooManyRequests, "查询过于频繁，请稍后再试")
		return
	}
	phone, ok := guestTrackPhone(r)
	if !ok {
		jsonError(w, http.StatusBadRequest, "请输入提交报修时使用的 11 位手机号")
		return
	}
	items, err := listGuestTickets(a.db, phone, 50)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	if items == nil {
		items = []Ticket{}
	}
	jsonResp(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

// apiMyTicketDetail GET /api/my/tickets/{id}?phone=…（公开）：工单详情 + 处理记录。
// 工单 phone 与查询不一致时按不存在处理，避免探测他人工单。
func (a *app) apiMyTicketDetail(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if !a.submitLimiter.allow(a.clientIP(r)) {
		jsonError(w, http.StatusTooManyRequests, "查询过于频繁，请稍后再试")
		return
	}
	phone, ok := guestTrackPhone(r)
	if !ok {
		jsonError(w, http.StatusBadRequest, "请输入提交报修时使用的 11 位手机号")
		return
	}
	t, err := getTicket(a.db, id)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	if t == nil || t.Phone != phone {
		http.NotFound(w, r)
		return
	}
	comments, err := listComments(a.db, id)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	if comments == nil {
		comments = []Comment{}
	}
	jsonResp(w, http.StatusOK, map[string]any{"data": map[string]any{
		"ticket":   t,
		"comments": comments,
	}})
}

// --------------------------------------------------------------------
// 单条工单：读取 / 编辑
// --------------------------------------------------------------------

func (a *app) apiTicketByID(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		a.apiTicketGet(w, r, id)
	case http.MethodPut:
		a.apiTicketUpdate(w, r, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *app) apiTicketGet(w http.ResponseWriter, r *http.Request, id int64) {
	t, err := getTicket(a.db, id)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	if t == nil {
		http.NotFound(w, r)
		return
	}
	jsonResp(w, http.StatusOK, map[string]any{"data": t})
}

func (a *app) apiTicketUpdate(w http.ResponseWriter, r *http.Request, id int64) {
	t, err := getTicket(a.db, id)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	if t == nil {
		http.NotFound(w, r)
		return
	}
	var body struct {
		Category string `json:"category"`
		Content  string `json:"content"`
		Name     string `json:"name"`  // 发起人姓名
		Phone    string `json:"phone"` // 发起人手机号
	}
	if err := decodeJSON(w, r, &body); err != nil {
		jsonError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	cat, content, name, phone, errMsg := validateTicketFields(a.db,
		strings.TrimSpace(body.Category),
		strings.TrimSpace(body.Content),
		strings.TrimSpace(body.Name),
		strings.TrimSpace(body.Phone),
	)
	if errMsg != "" {
		jsonError(w, http.StatusBadRequest, errMsg)
		return
	}
	if err := updateTicket(a.db, id, cat, content, name, phone); err != nil {
		jsonError(w, http.StatusInternalServerError, "保存失败")
		return
	}
	invalidateStatsCache()
	t, _ = getTicket(a.db, id)
	jsonResp(w, http.StatusOK, map[string]any{"data": t})
}

// --------------------------------------------------------------------
// 标记已处理（可选 note → 写备注）
// --------------------------------------------------------------------

func (a *app) apiTicketDone(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	t, err := getTicket(a.db, id)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	if t == nil {
		http.NotFound(w, r)
		return
	}

	var body struct {
		Note   string `json:"note"`
		Author string `json:"author"`
	}
	_ = decodeJSON(w, r, &body)

	if err := markDone(a.db, id); err != nil {
		jsonError(w, http.StatusInternalServerError, "操作失败")
		return
	}
	invalidateStatsCache()
	// 无论是否附解决备注，都写入一条处理记录：
	// 保证游客进度查询能看到「谁在何时完成了处理」
	author := strings.TrimSpace(body.Author)
	if author == "" {
		author = "系统"
	}
	content := "【标记已处理】"
	if note := strings.TrimSpace(body.Note); note != "" {
		content += note
	}
	if _, _, err := addComment(a.db, id, author, content); err != nil {
		log.Printf("写处理备注失败: %v", err)
	}
	t, _ = getTicket(a.db, id)
	jsonResp(w, http.StatusOK, map[string]any{"data": t})
}

// --------------------------------------------------------------------
// 删除
// --------------------------------------------------------------------

func (a *app) apiTicketDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	t, err := getTicket(a.db, id)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	if t == nil {
		http.NotFound(w, r)
		return
	}
	if err := deleteTicket(a.db, id); err != nil {
		jsonError(w, http.StatusInternalServerError, "删除失败")
		return
	}
	invalidateStatsCache()
	jsonResp(w, http.StatusOK, map[string]any{"ok": true})
}

// --------------------------------------------------------------------
// 备注 / 处理记录
// --------------------------------------------------------------------

func (a *app) apiTicketComments(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	t, err := getTicket(a.db, id)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	if t == nil {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		cs, err := listComments(a.db, id)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "查询失败")
			return
		}
		jsonResp(w, http.StatusOK, map[string]any{"items": cs})
	case http.MethodPost:
		var body struct {
			Author  string `json:"author"`
			Content string `json:"content"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			jsonError(w, http.StatusBadRequest, "请求体格式错误")
			return
		}
		author := strings.TrimSpace(body.Author)
		content := strings.TrimSpace(body.Content)
		if content == "" {
			jsonError(w, http.StatusBadRequest, "备注内容不能为空")
			return
		}
		if len([]rune(content)) > 1000 {
			jsonError(w, http.StatusBadRequest, "备注过长（最多 1000 字）")
			return
		}
		if author == "" {
			author = "匿名"
		}
		if len([]rune(author)) > 32 {
			jsonError(w, http.StatusBadRequest, "作者名过长（最多 32 字符）")
			return
		}
		cid, createdAt, err := addComment(a.db, id, author, content)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "添加失败")
			return
		}
		if err := touchTicket(a.db, id); err != nil {
			log.Printf("刷新工单 %d 更新时间失败: %v", id, err)
		}
		jsonResp(w, http.StatusCreated, map[string]any{"data": Comment{
			ID: cid, TicketID: id, Author: author, Content: content,
			CreatedAt: createdAt, // 与库中实际落库时间一致
		}})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --------------------------------------------------------------------
// 分类 CRUD
// --------------------------------------------------------------------

func (a *app) apiCategoryList(w http.ResponseWriter, r *http.Request) {
	cs, err := allCategories(a.db)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	jsonResp(w, http.StatusOK, map[string]any{"items": cs})
}

func (a *app) apiCategoryCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name  string `json:"name"`
		Color string `json:"color"`
		Sort  int    `json:"sort"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		jsonError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		jsonError(w, http.StatusBadRequest, "分类名不能为空")
		return
	}
	if len([]rune(name)) > 32 {
		jsonError(w, http.StatusBadRequest, "分类名过长（最多 32 字符）")
		return
	}
	color := strings.TrimSpace(body.Color)
	if color == "" {
		color = "#2563eb"
	}
	if !validColor(color) {
		jsonError(w, http.StatusBadRequest, "颜色格式须为 #RRGGBB")
		return
	}
	if c, _ := getCategoryByName(a.db, name); c != nil {
		jsonError(w, http.StatusBadRequest, "分类名已存在")
		return
	}
	_, err := a.db.Exec("INSERT INTO categories (name, color, sort) VALUES (?, ?, ?)", name, color, body.Sort)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "创建失败")
		return
	}
	c, _ := getCategoryByName(a.db, name)
	jsonResp(w, http.StatusCreated, map[string]any{"data": c})
}

func (a *app) apiCategoryByID(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodPut:
		a.apiCategoryUpdate(w, r, id)
	case http.MethodDelete:
		a.apiCategoryDelete(w, r, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// apiCategoryUpdate 部分更新：仅修改请求体中提供的字段，未提供字段保持不变。
func (a *app) apiCategoryUpdate(w http.ResponseWriter, r *http.Request, id int64) {
	var body struct {
		Name    *string `json:"name"`
		Color   *string `json:"color"`
		Sort    *int    `json:"sort"`
		Enabled *int    `json:"enabled"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		jsonError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	c, err := getCategoryByID(a.db, id)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	if c == nil {
		http.NotFound(w, r)
		return
	}

	name := c.Name
	if body.Name != nil {
		name = strings.TrimSpace(*body.Name)
		if name == "" {
			jsonError(w, http.StatusBadRequest, "分类名不能为空")
			return
		}
		if len([]rune(name)) > 32 {
			jsonError(w, http.StatusBadRequest, "分类名过长（最多 32 字符）")
			return
		}
		if other, _ := getCategoryByName(a.db, name); other != nil && other.ID != id {
			jsonError(w, http.StatusBadRequest, "分类名已存在")
			return
		}
	}
	color := c.Color
	if body.Color != nil {
		color = strings.TrimSpace(*body.Color)
		if !validColor(color) {
			jsonError(w, http.StatusBadRequest, "颜色格式须为 #RRGGBB")
			return
		}
	}
	sort := c.Sort
	if body.Sort != nil {
		sort = *body.Sort
	}
	enabled := c.Enabled
	if body.Enabled != nil {
		enabled = *body.Enabled
	}

	_, err = a.db.Exec("UPDATE categories SET name=?, color=?, sort=?, enabled=? WHERE id=?",
		name, color, sort, enabled, id)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "更新失败")
		return
	}
	c, _ = getCategoryByID(a.db, id)
	jsonResp(w, http.StatusOK, map[string]any{"data": c})
}

func (a *app) apiCategoryDelete(w http.ResponseWriter, r *http.Request, id int64) {
	res, err := a.db.Exec("DELETE FROM categories WHERE id=?", id)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "删除失败")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		http.NotFound(w, r)
		return
	}
	jsonResp(w, http.StatusOK, map[string]any{"ok": true})
}

// --------------------------------------------------------------------
// 指派负责人
// --------------------------------------------------------------------

// apiTicketAssign POST /api/tickets/{id}/assign —— 设置/清空工单负责人。
// body {"assignee": "<username>" 或 ""}；空串取消指派。
func (a *app) apiTicketAssign(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	t, err := getTicket(a.db, id)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	if t == nil {
		http.NotFound(w, r)
		return
	}
	var body struct {
		Assignee string `json:"assignee"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		jsonError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	assignee := strings.TrimSpace(body.Assignee)
	if len([]rune(assignee)) > 32 {
		jsonError(w, http.StatusBadRequest, "负责人用户名过长")
		return
	}
	if assignee != "" {
		u, _, err := getUserAuth(a.db, assignee)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "查询用户失败")
			return
		}
		if u == nil {
			jsonError(w, http.StatusBadRequest, "负责人不存在")
			return
		}
	}
	if err := assignTicket(a.db, id, assignee); err != nil {
		jsonError(w, http.StatusInternalServerError, "指派失败")
		return
	}
	// 写备注流水，保留指派痕迹
	action := "【取消指派】"
	if assignee != "" {
		action = "【指派】负责人：" + assignee
	}
	if _, _, err := addComment(a.db, id, "系统", action); err != nil {
		log.Printf("写指派备注失败: %v", err)
	}
	t, _ = getTicket(a.db, id)
	invalidateStatsCache()
	jsonResp(w, http.StatusOK, map[string]any{"data": t})
}

// --------------------------------------------------------------------
// 批量操作（批量标记已处理 / 批量删除）
// --------------------------------------------------------------------

const maxBatchIDs = 500

// parseBatchIDs 解析并校验批量操作的 id 列表：数量 1–500 且均为正整数。
func parseBatchIDs(w http.ResponseWriter, r *http.Request) ([]int64, string, bool) {
	var body struct {
		IDs    []int64 `json:"ids"`
		Author string  `json:"author"` // 仅 batch-done 使用：处理记录的作者
	}
	if err := decodeJSON(w, r, &body); err != nil {
		jsonError(w, http.StatusBadRequest, "请求体格式错误")
		return nil, "", false
	}
	if len(body.IDs) == 0 || len(body.IDs) > maxBatchIDs {
		jsonError(w, http.StatusBadRequest, fmt.Sprintf("ids 数量须为 1-%d", maxBatchIDs))
		return nil, "", false
	}
	for _, id := range body.IDs {
		if id <= 0 {
			jsonError(w, http.StatusBadRequest, "ids 含非法值")
			return nil, "", false
		}
	}
	author := strings.TrimSpace(body.Author)
	if author == "" {
		author = "系统"
	}
	return body.IDs, author, true
}

func (a *app) apiTicketBatchDone(w http.ResponseWriter, r *http.Request) {
	ids, author, ok := parseBatchIDs(w, r)
	if !ok {
		return
	}
	n, err := batchUpdateStatus(a.db, ids)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "批量操作失败")
		return
	}
	invalidateStatsCache()
	// 为每条实际更新的工单写入处理记录，与单个标记已处理保持一致，
	// 保证游客进度查询能看到完成轨迹
	existing, err := filterExistingTicketIDs(a.db, ids)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "批量操作失败")
		return
	}
	for _, id := range existing {
		if _, _, err := addComment(a.db, id, author, "【批量标记已处理】"); err != nil {
			log.Printf("写批量处理记录失败 (id=%d): %v", id, err)
		}
	}
	jsonResp(w, http.StatusOK, map[string]any{"data": map[string]any{"ok": true, "updated": n}})
}

func (a *app) apiTicketBatchDelete(w http.ResponseWriter, r *http.Request) {
	ids, _, ok := parseBatchIDs(w, r)
	if !ok {
		return
	}
	n, err := batchDeleteTickets(a.db, ids)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "批量删除失败")
		return
	}
	invalidateStatsCache()
	jsonResp(w, http.StatusOK, map[string]any{"data": map[string]any{"ok": true, "deleted": n}})
}

// --------------------------------------------------------------------
// CSV 导出
// --------------------------------------------------------------------

func (a *app) apiExportCSV(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	tq, ok := buildTicketQuery(q, 1, 100000)
	if !ok {
		jsonError(w, http.StatusBadRequest, "查询参数非法（status/from/to/order）")
		return
	}

	items, total, err := listTickets(a.db, tq)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	// 超过单次导出上限（10 万条）时通过响应头告知前端，避免静默截断
	if total > len(items) {
		w.Header().Set("X-Tix-Truncated", "1")
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=tix-%s.csv", time.Now().Format("20060102")))
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM，便于 Excel 正确识别中文

	cw := csv.NewWriter(w)
	defer cw.Flush()
	_ = cw.Write([]string{"编号", "分类", "内容", "发起人", "手机号", "状态", "创建时间", "更新时间"})
	statusLabel := map[int]string{0: "待处理", 1: "已处理"}
	for _, t := range items {
		_ = cw.Write([]string{
			csvSafe(ticketNumber(&t)), csvSafe(t.Category), csvSafe(t.Content), csvSafe(t.Creator), csvSafe(t.Phone),
			statusLabel[t.Status], t.CreatedAt, t.UpdatedAt,
		})
	}
}

// csvSafe 防御 CSV 公式注入：以 = + - @ \t \r 开头的单元格加单引号前缀。
func csvSafe(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}

// --------------------------------------------------------------------
// 工具函数
// --------------------------------------------------------------------

// clientIP 从请求中提取客户端IP。
// 仅在显式开启 -trust-proxy（部署于反向代理之后）时才信任
// X-Forwarded-For / X-Real-IP 头，否则直连客户端可伪造这些头绕过限流。
func (a *app) clientIP(r *http.Request) string {
	if a.trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// X-Forwarded-For: client, proxy1, proxy2
			if idx := strings.IndexByte(xff, ','); idx > 0 {
				return strings.TrimSpace(xff[:idx])
			}
			return xff
		}
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			return xri
		}
	}
	// 去掉端口号（兼容 IPv4 / IPv6，如 [::1]:8881 → ::1）
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// --------------------------------------------------------------------
// 字段校验
// --------------------------------------------------------------------

func validateTicketFields(db *sql.DB, category, content, name, phone string) (string, string, string, string, string) {
	if !validCategory(db, category) {
		return "", "", "", "", "分类不合法"
	}
	if content == "" {
		return "", "", "", "", "内容不能为空"
	}
	if len([]rune(content)) > 50 {
		return "", "", "", "", "内容过长（最多 50 字）"
	}
	if name == "" {
		return "", "", "", "", "发起人姓名不能为空"
	}
	if len([]rune(name)) > 20 {
		return "", "", "", "", "发起人姓名过长（最多 20 个字符）"
	}
	if phone == "" {
		return "", "", "", "", "请填写发起人手机号"
	}
	if !validPhone(phone) {
		return "", "", "", "", "请输入正确的 11 位手机号"
	}
	return category, content, name, phone, ""
}

// validPhone 校验 11 位大陆手机号（1 开头、第二位 3-9）。
func validPhone(p string) bool {
	if len(p) != 11 {
		return false
	}
	if p[0] != '1' || p[1] < '3' || p[1] > '9' {
		return false
	}
	for i := 2; i < 11; i++ {
		if p[i] < '0' || p[i] > '9' {
			return false
		}
	}
	return true
}

// validColor 校验十六进制颜色 #RRGGBB。
func validColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, ch := range s[1:] {
		if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f' || ch >= 'A' && ch <= 'F') {
			return false
		}
	}
	return true
}
