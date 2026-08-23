package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// ---------- Rate Limiter ----------

// rateLimiter 基于IP的简单限流器：每分钟最多N次请求。
type rateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limit    int
	window   time.Duration
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
}

// maxRateKeys 触发全表清扫的 key 数阈值：防止大量一次性 key
// （如伪造 X-Forwarded-For 的请求）导致内存无界增长。
const maxRateKeys = 4096

// allow 检查给定key在时间窗口内的请求是否超过限制。
func (r *rateLimiter) allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-r.window)

	// key 总量超阈值时整体清扫一次过期 key
	if len(r.requests) > maxRateKeys {
		for k, ts := range r.requests {
			if len(ts) == 0 || !ts[len(ts)-1].After(windowStart) {
				delete(r.requests, k)
			}
		}
	}

	// 清理过期记录
	timestamps := r.requests[key]
	valid := timestamps[:0]
	for _, t := range timestamps {
		if t.After(windowStart) {
			valid = append(valid, t)
		}
	}
	r.requests[key] = valid

	if len(valid) >= r.limit {
		return false
	}
	r.requests[key] = append(r.requests[key], now)
	return true
}

// submitLimiter 限制 /api/submit 端点：同IP每分钟最多10次。
var submitLimiter = newRateLimiter(10, time.Minute)

// loginLimiter 限制 /api/login 端点：同IP每分钟最多10次，缓解密码爆破。
var loginLimiter = newRateLimiter(10, time.Minute)

// ---------- 数据模型 ----------

// Ticket 一条工单记录。
type Ticket struct {
	ID        int64  `json:"id"`
	Category  string `json:"category"`
	Content   string `json:"content"`
	Creator   string `json:"creator"`
	Status    int    `json:"status"` // 0=待处理 1=已处理
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// Comment 一条处理记录 / 备注。
type Comment struct {
	ID        int64  `json:"id"`
	TicketID  int64  `json:"ticket_id"`
	Author    string `json:"author"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

// Category 分类元数据（可配置）。
type Category struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Color   string `json:"color"`
	Sort    int    `json:"sort"`
	Enabled int    `json:"enabled"` // 1 启用 0 停用
}

// Stats 统计看板数据。
type Stats struct {
	Pending  int           `json:"pending"`
	Done     int           `json:"done"`
	TodayNew int           `json:"today_new"`
	ByCat    []CatCount    `json:"by_cat"`
	ByDay    []CatCount    `json:"by_day"`
	ByDayCat []DayCatCount `json:"by_day_cat"`
	MonthCat []CatCount    `json:"month_cat"`
}

// CatCount 某维度下的数量。
type CatCount struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
}

// DayCatCount 某天某分类下的数量（按天 × 分类堆叠图用）。
type DayCatCount struct {
	Day      string `json:"day"`
	Category string `json:"category"`
	Count    int    `json:"count"`
}

// User 系统用户。
type User struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"` // admin / operator
	CreatedAt   string `json:"created_at"`
}

// ---------- 分类默认数据 ----------

var defaultCategories = []string{"硬件故障", "软件问题", "网络问题", "打印机故障", "其他"}

var defaultCategoryColors = map[string]string{
	"硬件故障":   "#f59e0b",
	"软件问题":   "#2563eb",
	"网络问题":   "#7c3aed",
	"打印机故障": "#10b981",
	"其他":      "#6b7280",
}

// ---------- 应用 ----------

// app 应用依赖：数据库、会话、统一推送。
type app struct {
	db         *sql.DB
	auth       *authStore
	notify     *notifier
	trustProxy bool // -trust-proxy：是否信任反向代理头（XFF/X-Real-IP）
}

func nowStr() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

func todayStr() string {
	return time.Now().Format("2006-01-02")
}

// ---------- 数据库打开与迁移 ----------

// openDB 打开 SQLite 数据库；单人使用固定单连接，避免偶发 database is locked。
func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("设置 %s 失败: %w", pragma, err)
		}
	}
	return db, nil
}

// initDB 创建基础表（幂等）。旧版本遗留的 priority 列由 migrateDB 清理。
func initDB(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS tickets (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	category   TEXT    NOT NULL,
	content    TEXT    NOT NULL,
	creator    TEXT    NOT NULL,
	status     INTEGER NOT NULL DEFAULT 0,
	created_at TEXT    NOT NULL,
	updated_at TEXT    NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_tickets_status ON tickets(status);
CREATE TABLE IF NOT EXISTS categories (
	id      INTEGER PRIMARY KEY AUTOINCREMENT,
	name    TEXT    NOT NULL UNIQUE,
	color   TEXT    NOT NULL DEFAULT '#2563eb',
	sort    INTEGER NOT NULL DEFAULT 0,
	enabled INTEGER NOT NULL DEFAULT 1
);
CREATE TABLE IF NOT EXISTS comments (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	ticket_id  INTEGER NOT NULL REFERENCES tickets(id),
	author     TEXT    NOT NULL,
	content    TEXT    NOT NULL,
	created_at TEXT    NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_comments_ticket ON comments(ticket_id);
CREATE TABLE IF NOT EXISTS users (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	username     TEXT    NOT NULL UNIQUE,
	password     TEXT    NOT NULL,
	display_name TEXT    NOT NULL,
	role         TEXT    NOT NULL DEFAULT 'operator',
	created_at   TEXT    NOT NULL
);
CREATE TABLE IF NOT EXISTS settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);`)
	return err
}

// hasColumn 检查某表是否已存在指定列。
func hasColumn(db *sql.DB, table, col string) (bool, error) {
	var n int
	err := db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM pragma_table_info('%s') WHERE name=?", table), col).Scan(&n)
	return n > 0, err
}

// migrateDB 事务化幂等迁移：删除旧版本遗留的 priority 列（已移除优先级设计）、初始化分类种子。
func migrateDB(db *sql.DB) error {
	has, err := hasColumn(db, "tickets", "priority")
	if err != nil {
		return err
	}
	if has {
		log.Printf("[迁移] 删除 tickets.priority 列（已移除优先级设计）")
		if _, err = db.Exec("ALTER TABLE tickets DROP COLUMN priority"); err != nil {
			return fmt.Errorf("删除 priority 列失败: %w", err)
		}
	}

	// 分类种子：仅当 categories 表为空时写入固定五类
	var seedCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM categories").Scan(&seedCount); err != nil {
		return err
	}
	if seedCount == 0 {
		log.Printf("[迁移] 初始化分类种子（%d 条）", len(defaultCategories))
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		for i, name := range defaultCategories {
			color := defaultCategoryColors[name]
			if color == "" {
				color = "#2563eb"
			}
			_, err = tx.Exec("INSERT INTO categories (name, color, sort, enabled) VALUES (?, ?, ?, 1)", name, color, i)
			if err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("初始化分类 %s 失败: %w", name, err)
			}
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}

	// 密码哈希迁移：旧版本明文密码在启动时一次性升级为 bcrypt 哈希
	if err := migratePlaintextPasswords(db); err != nil {
		return err
	}

	return nil
}

// migratePlaintextPasswords 将 users 表中残留的明文密码升级为 bcrypt。
// 判定标准：bcrypt 哈希以 $2 开头，其余视为明文（幂等：已迁移的行不再匹配）。
func migratePlaintextPasswords(db *sql.DB) error {
	rows, err := db.Query("SELECT id, password FROM users WHERE password NOT LIKE '$2%'")
	if err != nil {
		return err
	}
	type pair struct {
		id int64
		pw string
	}
	var legacy []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.id, &p.pw); err != nil {
			rows.Close()
			return err
		}
		legacy = append(legacy, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, p := range legacy {
		hash, err := hashPassword(p.pw)
		if err != nil {
			log.Printf("[迁移] 用户 %d 密码哈希失败，跳过: %v", p.id, err)
			continue
		}
		if _, err := db.Exec("UPDATE users SET password = ? WHERE id = ?", hash, p.id); err != nil {
			return fmt.Errorf("升级用户 %d 密码哈希失败: %w", p.id, err)
		}
	}
	if len(legacy) > 0 {
		log.Printf("[迁移] 已将 %d 个明文密码升级为 bcrypt 哈希", len(legacy))
	}
	return nil
}

// ---------- 分类数据访问 ----------

func allCategories(db *sql.DB) ([]Category, error) {
	rows, err := db.Query("SELECT id, name, color, sort, enabled FROM categories ORDER BY sort ASC, id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cs []Category
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.Name, &c.Color, &c.Sort, &c.Enabled); err != nil {
			return nil, err
		}
		cs = append(cs, c)
	}
	return cs, rows.Err()
}

func enabledCategoryNames(db *sql.DB) ([]string, error) {
	rows, err := db.Query("SELECT name FROM categories WHERE enabled = 1 ORDER BY sort ASC, id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		names = append(names, n)
	}
	return names, rows.Err()
}

func getCategoryByName(db *sql.DB, name string) (*Category, error) {
	c := &Category{}
	err := db.QueryRow("SELECT id, name, color, sort, enabled FROM categories WHERE name = ?", name).
		Scan(&c.ID, &c.Name, &c.Color, &c.Sort, &c.Enabled)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}

func getCategoryByID(db *sql.DB, id int64) (*Category, error) {
	c := &Category{}
	err := db.QueryRow("SELECT id, name, color, sort, enabled FROM categories WHERE id = ?", id).
		Scan(&c.ID, &c.Name, &c.Color, &c.Sort, &c.Enabled)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}

// validCategory 判断分类是否存在且已启用。
func validCategory(db *sql.DB, c string) bool {
	if c == "" {
		return false
	}
	var n int
	_ = db.QueryRow("SELECT COUNT(*) FROM categories WHERE name = ? AND enabled = 1", c).Scan(&n)
	return n > 0
}

// ---------- 工单数据访问 ----------

const ticketCols = "id, category, content, creator, status, created_at, updated_at"

// listTickets 查询工单：状态/分类/关键词可选；支持分页与排序。
func listTickets(db *sql.DB, status int, category, keyword string, page, size int, order string) ([]Ticket, int, error) {
	where := []string{}
	args := []any{}
	if status >= 0 {
		where = append(where, "status = ?")
		args = append(args, status)
	}
	if category != "" {
		where = append(where, "category = ?")
		args = append(args, category)
	}
	if keyword != "" {
		kw := "%" + escapeLike(keyword) + "%"
		where = append(where, "(content LIKE ? ESCAPE '/' OR creator LIKE ? ESCAPE '/')")
		args = append(args, kw, kw)
	}
	cond := ""
	if len(where) > 0 {
		cond = "WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := db.QueryRow("SELECT COUNT(*) FROM tickets "+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * size
	rows, err := db.Query("SELECT "+ticketCols+" FROM tickets "+cond+" ORDER BY id "+strings.ToUpper(order)+" LIMIT ? OFFSET ?",
		append(args, size, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var ts []Ticket
	for rows.Next() {
		var t Ticket
		if err := rows.Scan(&t.ID, &t.Category, &t.Content, &t.Creator, &t.Status, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, 0, err
		}
		ts = append(ts, t)
	}
	return ts, total, rows.Err()
}

// getTicket 查询单条工单；不存在时返回 (nil, nil)。
func getTicket(db *sql.DB, id int64) (*Ticket, error) {
	t := &Ticket{}
	err := db.QueryRow("SELECT "+ticketCols+" FROM tickets WHERE id = ?", id).
		Scan(&t.ID, &t.Category, &t.Content, &t.Creator, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return t, err
}

func createTicket(db *sql.DB, category, content, creator string) (int64, error) {
	now := nowStr()
	res, err := db.Exec(
		"INSERT INTO tickets (category, content, creator, status, created_at, updated_at) VALUES (?, ?, ?, 0, ?, ?)",
		category, content, creator, now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func updateTicket(db *sql.DB, id int64, category, content, creator string) error {
	_, err := db.Exec("UPDATE tickets SET category = ?, content = ?, creator = ?, updated_at = ? WHERE id = ?",
		category, content, creator, nowStr(), id)
	return err
}

func markDone(db *sql.DB, id int64) error {
	_, err := db.Exec("UPDATE tickets SET status = 1, updated_at = ? WHERE id = ?", nowStr(), id)
	return err
}

func deleteTicket(db *sql.DB, id int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM comments WHERE ticket_id = ?", id); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec("DELETE FROM tickets WHERE id = ?", id); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// ---------- 备注 / 处理记录 ----------

func addComment(db *sql.DB, ticketID int64, author, content string) (int64, error) {
	res, err := db.Exec("INSERT INTO comments (ticket_id, author, content, created_at) VALUES (?, ?, ?, ?)",
		ticketID, author, content, nowStr())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func listComments(db *sql.DB, ticketID int64) ([]Comment, error) {
	rows, err := db.Query("SELECT id, ticket_id, author, content, created_at FROM comments WHERE ticket_id = ? ORDER BY id ASC", ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cs []Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.TicketID, &c.Author, &c.Content, &c.CreatedAt); err != nil {
			return nil, err
		}
		cs = append(cs, c)
	}
	return cs, rows.Err()
}

// ---------- 工单编号 ----------

// ticketNumber 生成工单编号 T-YYYYMMDD-NNNN（如 T-20260818-0001）。
func ticketNumber(t *Ticket) string {
	date := strings.ReplaceAll(strings.SplitN(t.CreatedAt, " ", 2)[0], "-", "")
	return fmt.Sprintf("T-%s-%04d", date, t.ID)
}

// ---------- 统计 ----------

// statsCache 统计数据内存缓存：5秒内复用，避免频繁查询。
type statsCache struct {
	mu        sync.RWMutex
	data      *Stats
	updatedAt time.Time
}

var globalStatsCache = &statsCache{}

func invalidateStatsCache() {
	globalStatsCache.mu.Lock()
	globalStatsCache.data = nil
	globalStatsCache.mu.Unlock()
}

func getStats(db *sql.DB) (Stats, error) {
	globalStatsCache.mu.RLock()
	if globalStatsCache.data != nil && time.Since(globalStatsCache.updatedAt) < 5*time.Second {
		s := *globalStatsCache.data
		globalStatsCache.mu.RUnlock()
		return s, nil
	}
	globalStatsCache.mu.RUnlock()

	globalStatsCache.mu.Lock()
	defer globalStatsCache.mu.Unlock()

	if globalStatsCache.data != nil && time.Since(globalStatsCache.updatedAt) < 5*time.Second {
		return *globalStatsCache.data, nil
	}

	s, err := queryStats(db)
	if err != nil {
		return s, err
	}
	globalStatsCache.data = &s
	globalStatsCache.updatedAt = time.Now()
	return s, nil
}

func queryStats(db *sql.DB) (Stats, error) {
	var s Stats
	rows, err := db.Query("SELECT status, category, COUNT(*) FROM tickets GROUP BY status, category")
	if err != nil {
		return s, err
	}
	defer rows.Close()
	pendingMap := make(map[string]int)
	doneMap := make(map[string]int)
	for rows.Next() {
		var status int
		var cat string
		var n int
		if err := rows.Scan(&status, &cat, &n); err != nil {
			return s, err
		}
		if status == 0 {
			s.Pending += n
			pendingMap[cat] = n
		} else {
			s.Done += n
			doneMap[cat] = n
		}
	}
	if err := rows.Err(); err != nil {
		return s, err
	}

	// 分类分布（按已启用分类顺序；缺失补 0，历史分类名保留）
	cats, err := enabledCategoryNames(db)
	if err != nil {
		return s, err
	}
	typeCatMap := make(map[string]bool)
	for _, c := range cats {
		s.ByCat = append(s.ByCat, CatCount{Category: c, Count: pendingMap[c]})
		typeCatMap[c] = true
	}
	for c := range pendingMap {
		if !typeCatMap[c] {
			s.ByCat = append(s.ByCat, CatCount{Category: c, Count: pendingMap[c]})
		}
	}

	// 按天工单数量（最近 7 天，缺失补 0）
	const dayRange = 7
	startDay := time.Now().AddDate(0, 0, -(dayRange - 1)).Format("2006-01-02")
	dayRows, err := db.Query("SELECT substr(created_at, 1, 10) AS d, COUNT(*) FROM tickets WHERE created_at >= ? GROUP BY d", startDay+" 00:00:00")
	if err != nil {
		return s, err
	}
	dayMap := make(map[string]int)
	for dayRows.Next() {
		var d string
		var n int
		if err := dayRows.Scan(&d, &n); err != nil {
			_ = dayRows.Close()
			return s, err
		}
		dayMap[d] = n
	}
	_ = dayRows.Close()
	for i := dayRange - 1; i >= 0; i-- {
		d := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		s.ByDay = append(s.ByDay, CatCount{Category: d, Count: dayMap[d]})
	}

	// 按天 × 分类（最近 7 天，堆叠柱状图用；缺失天/分类补 0，保证与“按天”图对齐）
	dayCatRows, err := db.Query("SELECT substr(created_at, 1, 10) AS d, category, COUNT(*) FROM tickets WHERE created_at >= ? GROUP BY d, category", startDay+" 00:00:00")
	if err != nil {
		return s, err
	}
	defer dayCatRows.Close()
	type dayCatKey struct{ day, cat string }
	dayCatMap := make(map[dayCatKey]int)
	for dayCatRows.Next() {
		var d, cat string
		var n int
		if err := dayCatRows.Scan(&d, &cat, &n); err != nil {
			return s, err
		}
		dayCatMap[dayCatKey{d, cat}] = n
	}
	if err := dayCatRows.Err(); err != nil {
		return s, err
	}
	gridCats := cats
	gridCatSet := make(map[string]bool, len(cats))
	for _, c := range cats {
		gridCatSet[c] = true
	}
	for k := range dayCatMap {
		if !gridCatSet[k.cat] {
			gridCatSet[k.cat] = true
			gridCats = append(gridCats, k.cat)
		}
	}
	for i := dayRange - 1; i >= 0; i-- {
		d := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		for _, c := range gridCats {
			s.ByDayCat = append(s.ByDayCat, DayCatCount{Day: d, Category: c, Count: dayCatMap[dayCatKey{d, c}]})
		}
	}

	// 本月各分类工单数量（扇形图用）
	monthStart := time.Now().Format("2006-01") + "-01 00:00:00"
	monthRows, err := db.Query("SELECT category, COUNT(*) FROM tickets WHERE created_at >= ? GROUP BY category", monthStart)
	if err != nil {
		return s, err
	}
	defer monthRows.Close()
	for monthRows.Next() {
		var cat string
		var n int
		if err := monthRows.Scan(&cat, &n); err != nil {
			return s, err
		}
		s.MonthCat = append(s.MonthCat, CatCount{Category: cat, Count: n})
	}
	if err := monthRows.Err(); err != nil {
		return s, err
	}

	// 今日新增
	rows2, err := db.Query("SELECT COUNT(*) FROM tickets WHERE created_at >= ?", todayStr()+" 00:00:00")
	if err != nil {
		return s, err
	}
	defer rows2.Close()
	if rows2.Next() {
		_ = rows2.Scan(&s.TodayNew)
	}

	return s, nil
}

// escapeLike 转义 LIKE 通配符。
func escapeLike(s string) string {
	r := strings.NewReplacer("/", "//", "%", "/%", "_", "/_")
	return r.Replace(s)
}

// ---------- 设置 ----------

func getSetting(db *sql.DB, key string) (string, error) {
	var v string
	err := db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

func setSetting(db *sql.DB, key, value string) error {
	_, err := db.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)", key, value)
	return err
}

func getAllSettings(db *sql.DB) (map[string]string, error) {
	rows, err := db.Query("SELECT key, value FROM settings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		m[k] = v
	}
	return m, rows.Err()
}

// ---------- 用户数据访问 ----------

// validateUsername 校验用户名：仅允许字母、数字、下划线，长度3-32。
func validateUsername(username string) string {
	if len(username) < 3 || len(username) > 32 {
		return "用户名长度须为3-32个字符"
	}
	for _, ch := range username {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_') {
			return "用户名仅允许字母、数字和下划线"
		}
	}
	return ""
}

func getUserByUsername(db *sql.DB, username string) (*User, error) {
	u := &User{}
	err := db.QueryRow("SELECT id, username, display_name, role, created_at FROM users WHERE username = ?", username).
		Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

func getUserPassword(db *sql.DB, username string) (string, error) {
	var pw string
	err := db.QueryRow("SELECT password FROM users WHERE username = ?", username).Scan(&pw)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return pw, err
}

func getUserByID(db *sql.DB, id int64) (*User, error) {
	u := &User{}
	err := db.QueryRow("SELECT id, username, display_name, role, created_at FROM users WHERE id = ?", id).
		Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

func listUsers(db *sql.DB) ([]User, error) {
	rows, err := db.Query("SELECT id, username, display_name, role, created_at FROM users ORDER BY id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var us []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.CreatedAt); err != nil {
			return nil, err
		}
		us = append(us, u)
	}
	return us, rows.Err()
}

func createUser(db *sql.DB, username, password, displayName, role string) (int64, error) {
	hash, err := hashPassword(password)
	if err != nil {
		return 0, fmt.Errorf("密码哈希失败: %w", err)
	}
	now := nowStr()
	res, err := db.Exec(
		"INSERT INTO users (username, password, display_name, role, created_at) VALUES (?, ?, ?, ?, ?)",
		username, hash, displayName, role, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func updateUserPassword(db *sql.DB, id int64, password string) error {
	hash, err := hashPassword(password)
	if err != nil {
		return fmt.Errorf("密码哈希失败: %w", err)
	}
	_, err = db.Exec("UPDATE users SET password = ? WHERE id = ?", hash, id)
	return err
}

func updateUserProfile(db *sql.DB, id int64, displayName, role string) error {
	_, err := db.Exec("UPDATE users SET display_name = ?, role = ? WHERE id = ?", displayName, role, id)
	return err
}

func deleteUser(db *sql.DB, id int64) error {
	_, err := db.Exec("DELETE FROM users WHERE id = ?", id)
	return err
}

func setupDefaultAdmin(db *sql.DB, password string) error {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM users WHERE username = 'admin'").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	hash, err := hashPassword(password)
	if err != nil {
		return fmt.Errorf("密码哈希失败: %w", err)
	}
	_, err = db.Exec(
		"INSERT OR IGNORE INTO users (username, password, display_name, role, created_at) VALUES (?, ?, ?, ?, ?)",
		"admin", hash, "管理员", "admin", nowStr())
	return err
}
