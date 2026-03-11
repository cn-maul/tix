package database

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB
}

func Open(filename string) (*DB, error) {
	db, err := sql.Open("sqlite", filename)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return &DB{db}, nil
}

func (db *DB) InitSchema() error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS tickets (
			id TEXT PRIMARY KEY,
			initiator TEXT NOT NULL,
			category TEXT NOT NULL,
			title TEXT DEFAULT '',
			content TEXT NOT NULL,
			resolution TEXT DEFAULT '',
			is_completed BOOLEAN DEFAULT FALSE,
			created_at TEXT NOT NULL,
			completed_at TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_created_at ON tickets(created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_is_completed ON tickets(is_completed);
		CREATE INDEX IF NOT EXISTS idx_category ON tickets(category);
	`)
	if err != nil {
		return err
	}
	// 迁移：添加 title 字段
	db.Exec("ALTER TABLE tickets ADD COLUMN title TEXT DEFAULT ''")
	return nil
}

func (db *DB) CreateTicket(t *Ticket) error {
	_, err := db.Exec(`
		INSERT INTO tickets (id, initiator, category, title, content, resolution, is_completed, created_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, t.ID, t.Initiator, t.Category, t.Title, t.Content, t.Resolution, t.IsCompleted, t.CreatedAt, t.CompletedAt)
	return err
}

func (db *DB) GetTicket(id string) (*Ticket, error) {
	t := &Ticket{}
	var completedAt sql.NullString
	err := db.QueryRow(`
		SELECT id, initiator, category, title, content, resolution, is_completed, created_at, completed_at
		FROM tickets WHERE id = ?
	`, id).Scan(&t.ID, &t.Initiator, &t.Category, &t.Title, &t.Content, &t.Resolution, &t.IsCompleted, &t.CreatedAt, &completedAt)
	if err != nil {
		return nil, err
	}
	if completedAt.Valid {
		t.CompletedAt = &completedAt.String
	}
	return t, nil
}

func (db *DB) UpdateTicket(id string, category *string, resolution *string, isCompleted *bool, completedAt *string, createdAt *string) error {
	query := "UPDATE tickets SET "
	args := []any{}
	updates := []string{}

	if category != nil {
		updates = append(updates, "category = ?")
		args = append(args, *category)
	}
	if resolution != nil {
		updates = append(updates, "resolution = ?")
		args = append(args, *resolution)
	}
	if isCompleted != nil {
		updates = append(updates, "is_completed = ?")
		args = append(args, *isCompleted)
	}
	if completedAt != nil {
		updates = append(updates, "completed_at = ?")
		args = append(args, *completedAt)
	}
	if createdAt != nil {
		updates = append(updates, "created_at = ?")
		args = append(args, *createdAt)
	}

	if len(updates) == 0 {
		return nil
	}

	query += fmt.Sprintf("%s WHERE id = ?", joinUpdates(updates))
	args = append(args, id)

	_, err := db.Exec(query, args...)
	return err
}

func joinUpdates(updates []string) string {
	var result strings.Builder
	for i, u := range updates {
		if i > 0 {
			result.WriteString(", ")
		}
		result.WriteString(u)
	}
	return result.String()
}

func (db *DB) DeleteTicket(id string) error {
	_, err := db.Exec("DELETE FROM tickets WHERE id = ?", id)
	return err
}

func (db *DB) UpdateTicketCategory(oldName, newName string) error {
	_, err := db.Exec("UPDATE tickets SET category = ? WHERE category = ?", newName, oldName)
	return err
}

func (db *DB) TransferTicketCategory(from, to string) error {
	_, err := db.Exec("UPDATE tickets SET category = ? WHERE category = ?", to, from)
	return err
}

type ListOptions struct {
	Completed *bool
	Category  string
	Initiator string
	StartDate string
	EndDate   string
	Limit     int
	Offset    int
}

func (db *DB) ListTickets(opts ListOptions) ([]Ticket, int, error) {
	query := "SELECT id, initiator, category, title, content, resolution, is_completed, created_at, completed_at FROM tickets WHERE 1=1"
	countQuery := "SELECT COUNT(*) FROM tickets WHERE 1=1"
	args := []any{}

	if opts.Completed != nil {
		query += " AND is_completed = ?"
		countQuery += " AND is_completed = ?"
		args = append(args, *opts.Completed)
	}
	if opts.Category != "" {
		query += " AND category = ?"
		countQuery += " AND category = ?"
		args = append(args, opts.Category)
	}
	if opts.Initiator != "" {
		query += " AND initiator LIKE ?"
		countQuery += " AND initiator LIKE ?"
		args = append(args, "%"+opts.Initiator+"%")
	}
	if opts.StartDate != "" {
		query += " AND created_at >= ?"
		countQuery += " AND created_at >= ?"
		args = append(args, opts.StartDate+" 00:00:00")
	}
	if opts.EndDate != "" {
		query += " AND created_at <= ?"
		countQuery += " AND created_at <= ?"
		args = append(args, opts.EndDate+" 23:59:59")
	}

	var total int
	if err := db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query += " ORDER BY created_at DESC"
	if opts.Limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, opts.Limit, opts.Offset)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var tickets []Ticket
	for rows.Next() {
		var t Ticket
		var completedAt sql.NullString
		if err := rows.Scan(&t.ID, &t.Initiator, &t.Category, &t.Title, &t.Content, &t.Resolution, &t.IsCompleted, &t.CreatedAt, &completedAt); err != nil {
			return nil, 0, err
		}
		if completedAt.Valid {
			t.CompletedAt = &completedAt.String
		}
		tickets = append(tickets, t)
	}

	return tickets, total, nil
}

// Ticket 用于数据库操作
type Ticket struct {
	ID          string
	Initiator   string
	Category    string
	Title       string
	Content     string
	Resolution  string
	IsCompleted bool
	CreatedAt   string
	CompletedAt *string
}
