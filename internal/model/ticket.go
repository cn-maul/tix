package model

import "time"

type Ticket struct {
	ID          string     `json:"id"`
	Initiator   string     `json:"initiator"`
	Category    string     `json:"category"`
	Title       string     `json:"title"`
	Content     string     `json:"content"`
	Resolution  string     `json:"resolution"`
	IsCompleted bool       `json:"is_completed"`
	CreatedAt   string     `json:"created_at"`
	CompletedAt *string    `json:"completed_at"`
}

type CreateTicketReq struct {
	Initiator string `json:"initiator"`
	Category  string `json:"category"`
	Content   string `json:"content"`
}

type UpdateTicketReq struct {
	Category    *string `json:"category"`
	Resolution  *string `json:"resolution"`
	IsCompleted *bool   `json:"is_completed"`
	CreatedAt   *string `json:"created_at"`
	CompletedAt *string `json:"completed_at"`
}

type TicketListResp struct {
	Total int      `json:"total"`
	Items []Ticket `json:"items"`
}

type ErrorResp struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type CategoryResp struct {
	Categories []string `json:"categories"`
}

func NewTicket(initiator, category, title, content string) *Ticket {
	now := time.Now().Format(time.RFC3339)
	return &Ticket{
		ID:          generateUUID(),
		Initiator:   initiator,
		Category:    category,
		Title:       title,
		Content:     content,
		Resolution:  "",
		IsCompleted: false,
		CreatedAt:   now,
		CompletedAt: nil,
	}
}

func generateUUID() string {
	// 简单实现，实际用 google/uuid
	return "temp"
}
