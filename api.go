package main

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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
// 工单列表（分页 + 状态/分类/关键词筛选）
// --------------------------------------------------------------------

func (a *app) apiTicketList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	status := -1
	if v := q.Get("status"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || (n != 0 && n != 1) {
			jsonError(w, http.StatusBadRequest, "status 非法")
			return
		}
		status = n
	}

	category := q.Get("category")
	keyword := strings.TrimSpace(q.Get("keyword"))

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
	order := q.Get("order")
	if order == "" {
		order = "desc"
	}
	if !strings.EqualFold(order, "asc") && !strings.EqualFold(order, "desc") {
		jsonError(w, http.StatusBadRequest, "order 仅支持 asc/desc")
		return
	}

	items, total, err := listTickets(a.db, status, category, keyword, page, size, order)
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
		Creator  string `json:"creator"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		jsonError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	cat, content, creator, errMsg := validateTicketFields(a.db, body.Category, strings.TrimSpace(body.Content), strings.TrimSpace(body.Creator))
	if errMsg != "" {
		jsonError(w, http.StatusBadRequest, errMsg)
		return
	}
	id, err := createTicket(a.db, cat, content, creator)
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
	if !submitLimiter.allow(ip) {
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
	cat, content, creator, errMsg := validateTicketFields(a.db,
		r.FormValue("category"),
		strings.TrimSpace(r.FormValue("content")),
		strings.TrimSpace(r.FormValue("creator")),
	)
	if errMsg != "" {
		jsonError(w, http.StatusBadRequest, errMsg)
		return
	}
	id, err := createTicket(a.db, cat, content, creator)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "提交失败")
		return
	}
	invalidateStatsCache()
	t, _ := getTicket(a.db, id)
	jsonResp(w, http.StatusCreated, map[string]any{"data": t})
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
		Creator  string `json:"creator"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		jsonError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	cat, content, creator, errMsg := validateTicketFields(a.db,
		strings.TrimSpace(body.Category),
		strings.TrimSpace(body.Content),
		strings.TrimSpace(body.Creator),
	)
	if errMsg != "" {
		jsonError(w, http.StatusBadRequest, errMsg)
		return
	}
	if err := updateTicket(a.db, id, cat, content, creator); err != nil {
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
	if strings.TrimSpace(body.Note) != "" {
		author := strings.TrimSpace(body.Author)
		if author == "" {
			author = "系统"
		}
		if _, err := addComment(a.db, id, author, "【标记已处理】"+strings.TrimSpace(body.Note)); err != nil {
			log.Printf("写处理备注失败: %v", err)
		}
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
		cid, err := addComment(a.db, id, author, content)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "添加失败")
			return
		}
		_ = updateTicket(a.db, id, t.Category, t.Content, t.Creator) // 刷新 updated_at
		jsonResp(w, http.StatusCreated, map[string]any{"data": Comment{
			ID: cid, TicketID: id, Author: author, Content: content,
			CreatedAt: nowStr(),
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
// CSV 导出
// --------------------------------------------------------------------

func (a *app) apiExportCSV(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status := q.Get("status")
	category := q.Get("category")
	keyword := strings.TrimSpace(q.Get("keyword"))

	var st int
	if status == "" {
		st = -1
	} else {
		n, err := strconv.Atoi(status)
		if err != nil || (n != 0 && n != 1) {
			jsonError(w, http.StatusBadRequest, "status 非法")
			return
		}
		st = n
	}

	items, _, err := listTickets(a.db, st, category, keyword, 1, 100000, "desc")
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "查询失败")
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=tix-%s.csv", time.Now().Format("20060102")))
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM，便于 Excel 正确识别中文

	cw := csv.NewWriter(w)
	defer cw.Flush()
	_ = cw.Write([]string{"编号", "分类", "内容", "发起人", "状态", "创建时间", "更新时间"})
	statusLabel := map[int]string{0: "待处理", 1: "已处理"}
	for _, t := range items {
		_ = cw.Write([]string{
			csvSafe(ticketNumber(&t)), csvSafe(t.Category), csvSafe(t.Content), csvSafe(t.Creator),
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
	// 去掉端口号
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx > 0 {
		return addr[:idx]
	}
	return addr
}

// --------------------------------------------------------------------
// 字段校验
// --------------------------------------------------------------------

func validateTicketFields(db *sql.DB, category, content, creator string) (string, string, string, string) {
	if !validCategory(db, category) {
		return "", "", "", "分类不合法"
	}
	if content == "" {
		return "", "", "", "内容不能为空"
	}
	if len([]rune(content)) > 50 {
		return "", "", "", "内容过长（最多 50 字）"
	}
	if creator == "" {
		return "", "", "", "发起人不能为空"
	}
	if len([]rune(creator)) > 16 {
		return "", "", "", "发起人+手机号过长（最多 16 个字符）"
	}
	return category, content, creator, ""
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
