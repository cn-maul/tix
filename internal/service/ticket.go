package service

import (
	"context"
	"errors"
	"net/http"
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

func (s *TicketService) Create(ctx context.Context, req *model.CreateTicketReq) (*model.Ticket, error) {
	id := uuid.New().String()[:8]
	now := time.Now().Format(time.RFC3339)

	t := &model.Ticket{
		ID:          id,
		Initiator:   req.Initiator,
		Category:    req.Category,
		Content:     req.Content,
		Resolution:  "",
		IsCompleted: false,
		CreatedAt:   now,
		Priority:    req.Priority,
		Tags:        req.Tags,
	}

	// AI生成标题
	if s.ai != nil {
		title, err := s.ai.GenerateTitle(ctx, req.Content)
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

	if err := s.db.CreateTicket(ctx, t); err != nil {
		return nil, err
	}

	return t, nil
}

func (s *TicketService) Get(ctx context.Context, id string) (*model.Ticket, error) {
	return s.db.GetTicket(ctx, id)
}

func (s *TicketService) Update(ctx context.Context, id string, updates map[string]any) error {
	return s.db.UpdateTicket(ctx, id, updates)
}

func (s *TicketService) Delete(ctx context.Context, id string) error {
	return s.db.DeleteTicket(ctx, id)
}

func (s *TicketService) BatchDelete(ctx context.Context, ids []string) error {
	return s.db.BatchDelete(ctx, ids)
}

func (s *TicketService) BatchUpdate(ctx context.Context, ids []string, updates map[string]any) error {
	return s.db.BatchUpdate(ctx, ids, updates)
}

func (s *TicketService) List(ctx context.Context, opts database.ListOptions) (*model.ListResponse, error) {
	tickets, total, err := s.db.ListTickets(ctx, opts)
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

func (s *TicketService) ListRaw(ctx context.Context, opts database.ListOptions) ([]model.Ticket, int, error) {
	return s.db.ListTickets(ctx, opts)
}

func (s *TicketService) Import(ctx context.Context, tickets []model.Ticket) (int, int, error) {
	for i := range tickets {
		if tickets[i].ID == "" {
			tickets[i].ID = uuid.New().String()[:8]
		}
		if tickets[i].CreatedAt == "" {
			tickets[i].CreatedAt = time.Now().Format(time.RFC3339)
		}
		if tickets[i].Priority < 1 || tickets[i].Priority > 4 {
			tickets[i].Priority = model.PriorityNormal
		}
	}
	return s.db.ImportTickets(ctx, tickets)
}

func (s *TicketService) GetStats(ctx context.Context) (map[string]any, error) {
	return s.db.GetStats(ctx)
}

func (s *TicketService) GetInitiators(ctx context.Context, limit int) ([]string, error) {
	return s.db.GetInitiators(ctx, limit)
}

func (s *TicketService) TestAI(ctx context.Context, apiKey, baseURL, model string) (string, error) {
	var client *http.Client
	if s.ai != nil {
		client = s.ai.client
	}
	testAI := NewAIService(&config.AIConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   model,
	}, client)
	if testAI == nil {
		return "", errors.New("AI not configured")
	}
	return testAI.callAPI(ctx, "请回复'OK'")
}

func (s *TicketService) SelectCategory(ctx context.Context, content string, categories []string) (string, error) {
	if s.ai == nil {
		return "", errors.New("AI not configured")
	}
	// 动态创建 AI 服务，使用最新配置
	return s.ai.SelectCategoryFromList(ctx, content, categories)
}
