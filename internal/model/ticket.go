package model

type Ticket struct {
	ID          string  `json:"id"`
	Initiator   string  `json:"initiator"`
	Category    string  `json:"category"`
	Title       string  `json:"title"`
	Content     string  `json:"content"`
	Resolution  string  `json:"resolution"`
	IsCompleted bool    `json:"is_completed"`
	CreatedAt   string  `json:"created_at"`
	CompletedAt *string `json:"completed_at,omitempty"`
	Priority    int     `json:"priority"`
	Tags        string  `json:"tags"`
}

// 优先级常量
const (
	PriorityLow    = 1
	PriorityNormal = 2
	PriorityHigh   = 3
	PriorityUrgent = 4
)

type CreateTicketReq struct {
	Initiator string `json:"initiator"`
	Category  string `json:"category"`
	Content   string `json:"content"`
	Priority  int    `json:"priority"`
	Tags      string `json:"tags"`
}

type UpdateTicketReq struct {
	Category    *string `json:"category"`
	Resolution  *string `json:"resolution"`
	IsCompleted *bool   `json:"is_completed"`
	CompletedAt *string `json:"completed_at"`
	CreatedAt   *string `json:"created_at"`
	Priority    *int    `json:"priority"`
	Tags        *string `json:"tags"`
}

type BatchUpdateReq struct {
	IDs      []string        `json:"ids"`
	Updates  map[string]any  `json:"updates"`
}

type ListResponse struct {
	Items    []Ticket `json:"items"`
	Total    int      `json:"total"`
	Page     int      `json:"page"`
	PageSize int      `json:"page_size"`
}

type ErrorResp struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type StatsResponse struct {
	Total       int                    `json:"total"`
	Completed   int                    `json:"completed"`
	Pending     int                    `json:"pending"`
	Today       int                    `json:"today"`
	ThisWeek    int                    `json:"this_week"`
	ByCategory  []map[string]any       `json:"by_category"`
	ByPriority  []map[string]any       `json:"by_priority"`
}
