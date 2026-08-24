package main

import (
	"database/sql"
	"fmt"
	"log"
	"regexp"
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

// 限流器实例挂在 app 上（见 app.loginLimiter / app.submitLimiter），
// 每个应用实例独立计数，测试可按实例替换而互不干扰。

// ---------- 数据模型 ----------

// Ticket 一条工单记录。
type Ticket struct {
	ID        int64  `json:"id"`
	Category  string `json:"category"`
	Content   string `json:"content"`
	Creator   string `json:"creator"` // 发起人姓名
	Phone     string `json:"phone"`   // 发起人手机号（游客进度查询凭据）
	Status    int    `json:"status"`  // 0=待处理 1=已处理
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Assignee  string `json:"assignee"` // 负责人用户名，空串=未指派
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
	"硬件故障":  "#f59e0b",
	"软件问题":  "#2563eb",
	"网络问题":  "#7c3aed",
	"打印机故障": "#10b981",
	"其他":    "#6b7280",
}

// ---------- 应用 ----------

// app 应用依赖：数据库、会话、统一推送、请求限流。
type app struct {
	db            *sql.DB
	auth          *authStore
	notify        *notifier
	trustProxy    bool         // -trust-proxy：是否信任反向代理头（XFF/X-Real-IP）
	loginLimiter  *rateLimiter // /api/login 限流：同IP每分钟10次，缓解密码爆破
	submitLimiter *rateLimiter // /api/submit 限流：同IP每分钟10次
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
	phone      TEXT    NOT NULL DEFAULT '',
	status     INTEGER NOT NULL DEFAULT 0,
	created_at TEXT    NOT NULL,
	updated_at TEXT    NOT NULL,
	assignee   TEXT    NOT NULL DEFAULT ''
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

// migrateDB 事务化幂等迁移：删除旧版遗留 priority 列、补建 assignee 列、初始化分类种子。
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

	// 旧库补建 assignee（负责人）列；initDB 新建的表已含该列，此处幂等
	hasAssignee, err := hasColumn(db, "tickets", "assignee")
	if err != nil {
		return err
	}
	if !hasAssignee {
		log.Printf("[迁移] tickets 补建 assignee 列")
		if _, err = db.Exec("ALTER TABLE tickets ADD COLUMN assignee TEXT NOT NULL DEFAULT ''"); err != nil {
			return fmt.Errorf("补建 assignee 列失败: %w", err)
		}
	}

	// 旧库补建 phone（发起人手机号）列；initDB 新建的表已含该列，此处幂等
	hasPhone, err := hasColumn(db, "tickets", "phone")
	if err != nil {
		return err
	}
	if !hasPhone {
		log.Printf("[迁移] tickets 补建 phone 列")
		if _, err = db.Exec("ALTER TABLE tickets ADD COLUMN phone TEXT NOT NULL DEFAULT ''"); err != nil {
			return fmt.Errorf("补建 phone 列失败: %w", err)
		}
	}
	// 旧数据回填：早期版本把「姓名+手机号」拼在 creator 里，
	// 此处按尾部 11 位手机号自动拆分（幂等：仅处理 phone 仍为空的行）
	if err := migrateSplitCreatorPhone(db); err != nil {
		return err
	}

	// 旧数据补写完成记录：已处理但没有任何处理记录的工单（旧版标记已处理
	// 不强制写备注），游客端会显示「暂无处理记录」，补一条系统记录使状态可追溯
	if err := migrateBackfillDoneRecords(db); err != nil {
		return err
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

// legacyPhoneTailRe 匹配 creator 尾部的 11 位大陆手机号（旧版拼接格式）。
var legacyPhoneTailRe = regexp.MustCompile(`(1[3-9]\d{9})$`)

// migrateSplitCreatorPhone 把旧版「姓名+手机号」拼接的 creator 拆成姓名与 phone 两列。
// 幂等：只处理 phone 为空的行；拆分后姓名为空（整串即手机号）时保留原 creator 不动。
func migrateSplitCreatorPhone(db *sql.DB) error {
	rows, err := db.Query("SELECT id, creator FROM tickets WHERE phone = ''")
	if err != nil {
		return err
	}
	type pair struct {
		id         int64
		newCreator string // 拆分后的姓名；纯手机号行保持原 creator 不变
		phone      string
	}
	var legacy []pair
	for rows.Next() {
		var p pair
		var creator string
		if err := rows.Scan(&p.id, &creator); err != nil {
			rows.Close()
			return err
		}
		if m := legacyPhoneTailRe.FindStringSubmatch(creator); m != nil {
			p.phone = m[1]
			p.newCreator = strings.TrimSpace(strings.TrimSuffix(creator, m[1]))
			if p.newCreator == "" {
				// 整串就是一个手机号：手机号提取到独立列，creator 原样保留（列表仍有显示）
				p.newCreator = creator
			}
			legacy = append(legacy, p)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(legacy) == 0 {
		return nil
	}
	log.Printf("[迁移] 拆分 %d 条旧「姓名+手机号」拼接数据", len(legacy))
	for _, p := range legacy {
		if _, err := db.Exec("UPDATE tickets SET creator = ?, phone = ? WHERE id = ?", p.newCreator, p.phone, p.id); err != nil {
			return fmt.Errorf("回填 phone 失败 (id=%d): %w", p.id, err)
		}
	}
	return nil
}

// migrateBackfillDoneRecords 给「已处理但没有任何处理记录」的旧工单补一条
// 系统记录「【已处理完成】」。幂等：补写后工单即拥有记录，不再命中查询。
func migrateBackfillDoneRecords(db *sql.DB) error {
	rows, err := db.Query(
		"SELECT t.id FROM tickets t WHERE t.status = 1 AND " +
			"NOT EXISTS (SELECT 1 FROM comments c WHERE c.ticket_id = t.id)")
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	log.Printf("[迁移] 为 %d 条已处理工单补写完成记录", len(ids))
	for _, id := range ids {
		if _, _, err := addComment(db, id, "系统", "【已处理完成】"); err != nil {
			return fmt.Errorf("补写完成记录失败 (id=%d): %w", id, err)
		}
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

const ticketCols = "id, category, content, creator, phone, status, created_at, updated_at, assignee"

// ticketQuery 工单列表查询条件。
// From/To 为 YYYY-MM-DD 日期串（含边界日整天）；Assignee 为精确用户名；
// Unassigned 为真时只查未指派工单（优先于 Assignee）。
type ticketQuery struct {
	status     int // -1 = 全部
	category   string
	keyword    string
	from       string
	to         string
	assignee   string
	unassigned bool
	page       int
	size       int
	order      string
}

// listTickets 按查询条件分页返回工单与总数。
func listTickets(db *sql.DB, q ticketQuery) ([]Ticket, int, error) {
	where := []string{}
	args := []any{}
	if q.status >= 0 {
		where = append(where, "status = ?")
		args = append(args, q.status)
	}
	if q.category != "" {
		where = append(where, "category = ?")
		args = append(args, q.category)
	}
	if q.keyword != "" {
		kw := "%" + escapeLike(q.keyword) + "%"
		where = append(where, "(content LIKE ? ESCAPE '/' OR creator LIKE ? ESCAPE '/' OR phone LIKE ? ESCAPE '/')")
		args = append(args, kw, kw, kw)
	}
	if q.from != "" {
		where = append(where, "created_at >= ?")
		args = append(args, q.from+" 00:00:00")
	}
	if q.to != "" {
		where = append(where, "created_at <= ?")
		args = append(args, q.to+" 23:59:59")
	}
	if q.unassigned {
		where = append(where, "assignee = ''")
	} else if q.assignee != "" {
		where = append(where, "assignee = ?")
		args = append(args, q.assignee)
	}
	cond := ""
	if len(where) > 0 {
		cond = "WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := db.QueryRow("SELECT COUNT(*) FROM tickets "+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (q.page - 1) * q.size
	rows, err := db.Query("SELECT "+ticketCols+" FROM tickets "+cond+" ORDER BY id "+strings.ToUpper(q.order)+" LIMIT ? OFFSET ?",
		append(args, q.size, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var ts []Ticket
	for rows.Next() {
		var t Ticket
		if err := rows.Scan(&t.ID, &t.Category, &t.Content, &t.Creator, &t.Phone, &t.Status, &t.CreatedAt, &t.UpdatedAt, &t.Assignee); err != nil {
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
		Scan(&t.ID, &t.Category, &t.Content, &t.Creator, &t.Phone, &t.Status, &t.CreatedAt, &t.UpdatedAt, &t.Assignee)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return t, err
}

// assignTicket 更新工单负责人（空串表示取消指派）并刷新更新时间。
func assignTicket(db *sql.DB, id int64, assignee string) error {
	_, err := db.Exec("UPDATE tickets SET assignee = ?, updated_at = ? WHERE id = ?", assignee, nowStr(), id)
	return err
}

// listGuestTickets 游客进度查询：按手机号精确匹配（独立 phone 列），新的在前。
// limit 封顶返回条数，防止公开接口被拉取全量数据。
func listGuestTickets(db *sql.DB, phone string, limit int) ([]Ticket, error) {
	rows, err := db.Query(
		"SELECT "+ticketCols+" FROM tickets WHERE phone = ? ORDER BY id DESC LIMIT ?",
		phone, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ts []Ticket
	for rows.Next() {
		var t Ticket
		if err := rows.Scan(&t.ID, &t.Category, &t.Content, &t.Creator, &t.Phone, &t.Status, &t.CreatedAt, &t.UpdatedAt, &t.Assignee); err != nil {
			return nil, err
		}
		ts = append(ts, t)
	}
	return ts, rows.Err()
}

// batchUpdateStatus 批量标记已处理：只影响列表中实际存在的工单，返回受影响行数。
func batchUpdateStatus(db *sql.DB, ids []int64) (int64, error) {
	ph := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids)+1)
	args = append(args, nowStr())
	for _, id := range ids {
		args = append(args, id)
	}
	res, err := db.Exec("UPDATE tickets SET status = 1, updated_at = ? WHERE id IN ("+ph+")", args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func createTicket(db *sql.DB, category, content, name, phone string) (int64, error) {
	now := nowStr()
	res, err := db.Exec(
		"INSERT INTO tickets (category, content, creator, phone, status, created_at, updated_at) VALUES (?, ?, ?, ?, 0, ?, ?)",
		category, content, name, phone, now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func updateTicket(db *sql.DB, id int64, category, content, name, phone string) error {
	_, err := db.Exec("UPDATE tickets SET category = ?, content = ?, creator = ?, phone = ?, updated_at = ? WHERE id = ?",
		category, content, name, phone, nowStr(), id)
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

// filterExistingTicketIDs 返回 ids 中实际存在的工单 id（保持传入顺序、去重）。
func filterExistingTicketIDs(db *sql.DB, ids []int64) ([]int64, error) {
	ph := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := db.Query("SELECT id FROM tickets WHERE id IN ("+ph+")", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	exist := make(map[int64]bool)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		exist[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(exist))
	seen := make(map[int64]bool)
	for _, id := range ids {
		if exist[id] && !seen[id] {
			out = append(out, id)
			seen[id] = true
		}
	}
	return out, nil
}

// batchDeleteTickets 事务批量删除工单及其备注：只删除实际存在的工单，返回受影响行数。
func batchDeleteTickets(db *sql.DB, ids []int64) (int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	ph := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	if _, err := tx.Exec("DELETE FROM comments WHERE ticket_id IN ("+ph+")", args...); err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	res, err := tx.Exec("DELETE FROM tickets WHERE id IN ("+ph+")", args...)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, tx.Commit()
}

// ---------- 备注 / 处理记录 ----------

func addComment(db *sql.DB, ticketID int64, author, content string) (int64, string, error) {
	created := nowStr()
	res, err := db.Exec("INSERT INTO comments (ticket_id, author, content, created_at) VALUES (?, ?, ?, ?)",
		ticketID, author, content, created)
	if err != nil {
		return 0, "", err
	}
	id, err := res.LastInsertId()
	return id, created, err
}

// touchTicket 仅刷新 updated_at：备注等不修改工单正文的操作使用，
// 避免用请求前的旧值回写覆盖并发的编辑。
func touchTicket(db *sql.DB, id int64) error {
	_, err := db.Exec("UPDATE tickets SET updated_at = ? WHERE id = ?", nowStr(), id)
	return err
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

// getUserAuth 登录用单次查询：用户信息 + 密码哈希；不存在时返回 (nil, "", nil)。
func getUserAuth(db *sql.DB, username string) (*User, string, error) {
	u := &User{}
	var pw string
	err := db.QueryRow("SELECT id, username, display_name, role, created_at, password FROM users WHERE username = ?", username).
		Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.CreatedAt, &pw)
	if err == sql.ErrNoRows {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	return u, pw, nil
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
