package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"tix/internal/model"
)

type DB struct {
	*sql.DB
}

func Open(filename string) (*DB, error) {
	db, err := sql.Open("sqlite", filename)
	if err != nil {
		return nil, err
	}

	// 连接池配置
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)

	// SQLite PRAGMA 优化
	pragmas := []string{
		"PRAGMA journal_mode = WAL",   // 写前日志，提升并发
		"PRAGMA synchronous = NORMAL", // 平衡性能和安全
		"PRAGMA cache_size = -64000",  // 64MB 缓存
		"PRAGMA busy_timeout = 5000",  // 5秒超时
		"PRAGMA foreign_keys = ON",    // 外键约束
		"PRAGMA temp_store = MEMORY",  // 临时表在内存
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return nil, fmt.Errorf("pragma error: %v", err)
		}
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}
	return &DB{db}, nil
}

func (db *DB) InitSchema(ctx context.Context) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS tickets (
			id TEXT PRIMARY KEY,
			initiator TEXT NOT NULL,
			category TEXT NOT NULL,
			title TEXT DEFAULT '',
			content TEXT NOT NULL,
			resolution TEXT DEFAULT '',
			is_completed BOOLEAN DEFAULT FALSE,
			created_at TEXT NOT NULL,
			completed_at TEXT,
			priority INTEGER DEFAULT 2,
			tags TEXT DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_created_at ON tickets(created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_is_completed ON tickets(is_completed);
		CREATE INDEX IF NOT EXISTS idx_category ON tickets(category);
		CREATE INDEX IF NOT EXISTS idx_priority ON tickets(priority);
		CREATE INDEX IF NOT EXISTS idx_initiator ON tickets(initiator);
		CREATE INDEX IF NOT EXISTS idx_category_completed ON tickets(category, is_completed);
	`)
	if err != nil {
		return err
	}

	// 迁移：添加字段
	migrations := []string{
		"ALTER TABLE tickets ADD COLUMN title TEXT DEFAULT ''",
		"ALTER TABLE tickets ADD COLUMN priority INTEGER DEFAULT 2",
		"ALTER TABLE tickets ADD COLUMN tags TEXT DEFAULT ''",
	}
	for _, m := range migrations {
		db.ExecContext(ctx, m) // 忽略已存在错误
	}

	return nil
}

func (db *DB) CreateTicket(ctx context.Context, t *model.Ticket) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO tickets (id, initiator, category, title, content, resolution, is_completed, created_at, completed_at, priority, tags)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, t.ID, t.Initiator, t.Category, t.Title, t.Content, t.Resolution, t.IsCompleted, t.CreatedAt, t.CompletedAt, t.Priority, t.Tags)
	return err
}

func (db *DB) ImportTickets(ctx context.Context, tickets []model.Ticket) (int, int, error) {
	if len(tickets) == 0 {
		return 0, 0, nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR IGNORE INTO tickets (id, initiator, category, title, content, resolution, is_completed, created_at, completed_at, priority, tags)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return 0, 0, err
	}
	defer stmt.Close()

	imported := 0
	skipped := 0
	for _, t := range tickets {
		result, err := stmt.ExecContext(ctx, t.ID, t.Initiator, t.Category, t.Title, t.Content, t.Resolution, t.IsCompleted, t.CreatedAt, t.CompletedAt, t.Priority, t.Tags)
		if err != nil {
			return imported, skipped, err
		}

		rows, err := result.RowsAffected()
		if err != nil {
			return imported, skipped, err
		}
		if rows == 0 {
			skipped++
			continue
		}
		imported++
	}

	if err := tx.Commit(); err != nil {
		return imported, skipped, err
	}
	return imported, skipped, nil
}

func (db *DB) GetTicket(ctx context.Context, id string) (*model.Ticket, error) {
	t := &model.Ticket{}
	var completedAt sql.NullString
	err := db.QueryRowContext(ctx, `
		SELECT id, initiator, category, title, content, resolution, is_completed, created_at, completed_at, COALESCE(priority, 2), COALESCE(tags, '')
		FROM tickets WHERE id = ?
	`, id).Scan(&t.ID, &t.Initiator, &t.Category, &t.Title, &t.Content, &t.Resolution, &t.IsCompleted, &t.CreatedAt, &completedAt, &t.Priority, &t.Tags)
	if err != nil {
		return nil, err
	}
	if completedAt.Valid {
		t.CompletedAt = &completedAt.String
	}
	return t, nil
}

type ListOptions struct {
	Limit     int
	Offset    int
	Category  string
	Completed *bool
	Search    string
	Priority  int
	StartDate string
	EndDate   string
	Initiator string
	SortBy    string
	SortDesc  bool
}

var allowedSortFields = map[string]string{
	"created_at":   "created_at",
	"priority":     "priority",
	"initiator":    "initiator",
	"category":     "category",
	"is_completed": "is_completed",
}

func (db *DB) ListTickets(ctx context.Context, opts ListOptions) ([]model.Ticket, int, error) {
	// 构建查询条件
	conditions := []string{"1=1"}
	args := []any{}

	if opts.Category != "" {
		conditions = append(conditions, "category = ?")
		args = append(args, opts.Category)
	}
	if opts.Completed != nil {
		conditions = append(conditions, "is_completed = ?")
		args = append(args, *opts.Completed)
	}
	if opts.Search != "" {
		conditions = append(conditions, "(initiator LIKE ? OR content LIKE ? OR title LIKE ?)")
		search := "%" + opts.Search + "%"
		args = append(args, search, search, search)
	}
	if opts.Priority > 0 {
		conditions = append(conditions, "priority = ?")
		args = append(args, opts.Priority)
	}
	if opts.StartDate != "" {
		conditions = append(conditions, "created_at >= ?")
		args = append(args, opts.StartDate+"T00:00:00Z")
	}
	if opts.EndDate != "" {
		conditions = append(conditions, "created_at <= ?")
		args = append(args, opts.EndDate+"T23:59:59Z")
	}
	if opts.Initiator != "" {
		conditions = append(conditions, "initiator = ?")
		args = append(args, opts.Initiator)
	}

	whereClause := strings.Join(conditions, " AND ")

	// 排序
	sortBy := "created_at"
	if field, ok := allowedSortFields[opts.SortBy]; ok {
		sortBy = field
	}
	sortOrder := "DESC"
	if opts.SortBy != "" || opts.SortDesc {
		if !opts.SortDesc {
			sortOrder = "ASC"
		}
	}

	// 计数
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM tickets WHERE %s", whereClause)
	if err := db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// 查询列表
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	query := fmt.Sprintf(`
		SELECT id, initiator, category, title, content, resolution, is_completed, created_at, completed_at, COALESCE(priority, 2), COALESCE(tags, '')
		FROM tickets WHERE %s
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, whereClause, sortBy, sortOrder)

	args = append(args, limit, opts.Offset)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	tickets := make([]model.Ticket, 0, min(limit, total))
	for rows.Next() {
		var t model.Ticket
		var completedAt sql.NullString
		if err := rows.Scan(&t.ID, &t.Initiator, &t.Category, &t.Title, &t.Content, &t.Resolution, &t.IsCompleted, &t.CreatedAt, &completedAt, &t.Priority, &t.Tags); err != nil {
			return nil, 0, err
		}
		if completedAt.Valid {
			t.CompletedAt = &completedAt.String
		}
		tickets = append(tickets, t)
	}

	return tickets, total, rows.Err()
}

func (db *DB) DeleteTicket(ctx context.Context, id string) error {
	result, err := db.ExecContext(ctx, "DELETE FROM tickets WHERE id = ?", id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (db *DB) BatchDelete(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf("DELETE FROM tickets WHERE id IN (%s)", strings.Join(placeholders, ","))
	_, err := db.ExecContext(ctx, query, args...)
	return err
}

func (db *DB) BatchUpdate(ctx context.Context, ids []string, updates map[string]any) error {
	if len(ids) == 0 || len(updates) == 0 {
		return nil
	}

	setClauses := []string{}
	args := []any{}
	for k, v := range updates {
		setClauses = append(setClauses, k+" = ?")
		args = append(args, v)
	}

	placeholders := make([]string, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}

	query := fmt.Sprintf("UPDATE tickets SET %s WHERE id IN (%s)", strings.Join(setClauses, ","), strings.Join(placeholders, ","))
	_, err := db.ExecContext(ctx, query, args...)
	return err
}

func (db *DB) GetStats(ctx context.Context) (map[string]any, error) {
	stats := make(map[string]any)

	// 总数
	var total, completed int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tickets").Scan(&total)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tickets WHERE is_completed = 1").Scan(&completed)
	stats["total"] = total
	stats["completed"] = completed
	stats["pending"] = total - completed

	// 今日
	var today int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tickets WHERE date(created_at) = date('now')").Scan(&today)
	stats["today"] = today

	// 本周
	var thisWeek int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tickets WHERE created_at >= date('now', '-7 days')").Scan(&thisWeek)
	stats["this_week"] = thisWeek

	// 分类统计
	rows, err := db.QueryContext(ctx, "SELECT category, COUNT(*) as cnt FROM tickets GROUP BY category ORDER BY cnt DESC")
	if err == nil {
		defer rows.Close()
		categories := []map[string]any{}
		for rows.Next() {
			var cat string
			var cnt int
			if rows.Scan(&cat, &cnt) == nil {
				categories = append(categories, map[string]any{"category": cat, "count": cnt})
			}
		}
		stats["by_category"] = categories
	}

	// 优先级统计
	rows2, err := db.QueryContext(ctx, "SELECT priority, COUNT(*) as cnt FROM tickets GROUP BY priority ORDER BY priority")
	if err == nil {
		defer rows2.Close()
		priorities := []map[string]any{}
		for rows2.Next() {
			var pri, cnt int
			if rows2.Scan(&pri, &cnt) == nil {
				priorities = append(priorities, map[string]any{"priority": pri, "count": cnt})
			}
		}
		stats["by_priority"] = priorities
	}

	return stats, nil
}

func (db *DB) GetInitiators(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.QueryContext(ctx, "SELECT DISTINCT initiator FROM tickets ORDER BY initiator LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var initiators []string
	for rows.Next() {
		var i string
		if err := rows.Scan(&i); err != nil {
			return nil, err
		}
		initiators = append(initiators, i)
	}
	return initiators, rows.Err()
}

func (db *DB) UpdateTicket(ctx context.Context, id string, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}

	setClauses := make([]string, 0, len(updates))
	args := make([]any, 0, len(updates)+1)
	for k, v := range updates {
		setClauses = append(setClauses, k+" = ?")
		args = append(args, v)
	}
	args = append(args, id)

	query := fmt.Sprintf("UPDATE tickets SET %s WHERE id = ?", strings.Join(setClauses, ", "))
	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func IsNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
