package service

import (
	"database/sql"
	"errors"
	"time"

	"tix/internal/config"
	"tix/internal/database"
	"tix/internal/model"

	"github.com/google/uuid"
)

type TicketService struct {
	db *database.DB
	ai *AIService
}

func NewTicketService(db *database.DB) *TicketService {
	return &TicketService{db: db}
}

func (s *TicketService) SetAI(ai *AIService) {
	s.ai = ai
}

func (s *TicketService) Create(req *model.CreateTicketReq) (*model.Ticket, error) {
	id := uuid.New().String()[:8]
	now := time.Now().Format(time.RFC3339)

	t := &model.Ticket{
		ID:         id,
		Initiator:  req.Initiator,
		Category:   req.Category,
		Content:    req.Content,
		Resolution: "",
		IsCompleted: false,
		CreatedAt:  now,
		Priority:   req.Priority,
		Tags:       req.Tags,
	}

	// AI生成标题
	if s.ai != nil {
		title, err := s.ai.GenerateTitle(req.Content)
		if err == nil && title != "" {
			t.Title = title
		}
	}
	if t.Title == "" {
		runes := []rune(req.Content)
		if len(runes) > 15 {
			t.Title = string(runes[:15])
		} else {
			t.Title = req.Content
		}
	}

	// AI选择分类
	if t.Category == "" && s.ai != nil {
		// 这里需要从某处获取分类列表
		// 暂时留空
	}

	if t.Priority < 1 || t.Priority > 4 {
		t.Priority = model.PriorityNormal
	}

	if err := s.db.CreateTicket(t); err != nil {
		return nil, err
	}

	return t, nil
}

func (s *TicketService) Get(id string) (*model.Ticket, error) {
	return s.db.GetTicket(id)
}

func (s *TicketService) Update(id string, updates map[string]any) error {
	return s.db.UpdateTicket(id, updates)
}

func (s *TicketService) Delete(id string) error {
	return s.db.DeleteTicket(id)
}

func (s *TicketService) BatchDelete(ids []string) error {
	return s.db.BatchDelete(ids)
}

func (s *TicketService) BatchUpdate(ids []string, updates map[string]any) error {
	return s.db.BatchUpdate(ids, updates)
}

func (s *TicketService) List(opts database.ListOptions) (*model.ListResponse, error) {
	tickets, total, err := s.db.ListTickets(opts)
	if err != nil {
		return nil, err
	}

	page := 1
	if opts.Offset > 0 && opts.Limit > 0 {
		page = opts.Offset/opts.Limit + 1
	}

	return &model.ListResponse{
		Items:    tickets,
		Total:    total,
		Page:     page,
		PageSize: opts.Limit,
	}, nil
}

func (s *TicketService) ListRaw(opts database.ListOptions) ([]model.Ticket, int, error) {
	return s.db.ListTickets(opts)
}

func (s *TicketService) Import(tickets []model.Ticket) (int, int, error) {
	imported := 0
	skipped := 0

	for _, t := range tickets {
		_, err := s.db.GetTicket(t.ID)
		if err == nil {
			skipped++
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return imported, skipped, err
		}

		if t.ID == "" {
			t.ID = uuid.New().String()[:8]
		}
		if t.CreatedAt == "" {
			t.CreatedAt = time.Now().Format(time.RFC3339)
		}
		if t.Priority < 1 || t.Priority > 4 {
			t.Priority = model.PriorityNormal
		}

		if err := s.db.CreateTicket(&t); err != nil {
			skipped++
			continue
		}
		imported++
	}

	return imported, skipped, nil
}

func (s *TicketService) GetStats() (map[string]any, error) {
	return s.db.GetStats()
}

func (s *TicketService) GetInitiators(limit int) ([]string, error) {
	return s.db.GetInitiators(limit)
}

func (s *TicketService) TestAI(apiKey, baseURL, model string) (string, error) {
	testAI := &AIService{cfg: &config.AIConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   model,
	}}
	return testAI.callAPI("请回复'OK'")
}

func (s *TicketService) SelectCategory(content string, categories []string) (string, error) {
	// 动态创建 AI 服务，使用最新配置
	return s.ai.SelectCategoryFromList(content, categories)
}
