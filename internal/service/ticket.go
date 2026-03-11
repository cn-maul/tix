package service

import (
	"tix/internal/config"
	"tix/internal/database"
	"tix/internal/model"
	"log"
	"time"

	"github.com/google/uuid"
)

type TicketService struct {
	db         *database.DB
	categories []string
	cfg        *config.Config
}

func NewTicketService(db *database.DB, categories []string, cfg *config.Config) *TicketService {
	return &TicketService{db: db, categories: categories, cfg: cfg}
}

func (s *TicketService) Create(req *model.CreateTicketReq) (*model.Ticket, error) {
	id := uuid.New()
	uid := id.String()
	now := time.Now().Format(time.RFC3339)

	// AI分析生成标题和分类
	ai := NewAIClient(s.cfg.AI.APIKey, s.cfg.AI.BaseURL, s.cfg.AI.Model)
	aiResult := ai.AnalyzeContent(req.Content, s.categories)

	// 如果用户指定了分类，使用用户的；否则用AI选择的
	category := req.Category
	if category == "" {
		category = aiResult.Category
	}

	t := &database.Ticket{
		ID:          uid,
		Initiator:   req.Initiator,
		Category:    category,
		Title:       aiResult.Title,
		Content:     req.Content,
		Resolution:  "",
		IsCompleted: false,
		CreatedAt:   now,
		CompletedAt: nil,
	}

	if err := s.db.CreateTicket(t); err != nil {
		return nil, err
	}

	log.Printf("✓ 新建工单 #%s [%s] %s - %s", uid[:8], category, aiResult.Title, req.Initiator)
	return s.toModel(t), nil
}

func (s *TicketService) CreateWithID(id, createdAt string, req *model.CreateTicketReq) error {
	ai := NewAIClient(s.cfg.AI.APIKey, s.cfg.AI.BaseURL, s.cfg.AI.Model)
	aiResult := ai.AnalyzeContent(req.Content, s.categories)
	
	category := req.Category
	if category == "" {
		category = aiResult.Category
	}
	
	t := &database.Ticket{
		ID:          id,
		Initiator:   req.Initiator,
		Category:    category,
		Title:       aiResult.Title,
		Content:     req.Content,
		Resolution:  "",
		IsCompleted: false,
		CreatedAt:   createdAt,
		CompletedAt: nil,
	}
	return s.db.CreateTicket(t)
}

func (s *TicketService) Get(id string) (*model.Ticket, error) {
	t, err := s.db.GetTicket(id)
	if err != nil {
		return nil, err
	}
	return s.toModel(t), nil
}

func (s *TicketService) Update(id string, req *model.UpdateTicketReq) (*model.Ticket, error) {
	var resolution *string
	var completedAt *string
	var category *string
	var createdAt *string

	if req.Resolution != nil {
		resolution = req.Resolution
	}

	if req.Category != nil {
		category = req.Category
	}

	// 如果显式设置了完成时间，使用设置的值
	if req.CompletedAt != nil {
		completedAt = req.CompletedAt
	} else if req.IsCompleted != nil {
		// 否则按原来的逻辑：标记完成时自动设置当前时间
		if *req.IsCompleted {
			now := time.Now().Format(time.RFC3339)
			completedAt = &now
			log.Printf("✓ 完成工单 #%s", id[:8])
		} else {
			completedAt = nil // 需要在数据库层面处理
		}
	}

	if req.CreatedAt != nil {
		createdAt = req.CreatedAt
	}

	if err := s.db.UpdateTicket(id, category, resolution, req.IsCompleted, completedAt, createdAt); err != nil {
		return nil, err
	}

	return s.Get(id)
}

func (s *TicketService) Delete(id string) error {
	return s.db.DeleteTicket(id)
}

func (s *TicketService) UpdateCategory(oldName, newName string) error {
	return s.db.UpdateTicketCategory(oldName, newName)
}

func (s *TicketService) TransferCategory(from, to string) error {
	return s.db.TransferTicketCategory(from, to)
}

func (s *TicketService) List(opts database.ListOptions) (*model.TicketListResp, error) {
	tickets, total, err := s.db.ListTickets(opts)
	if err != nil {
		return nil, err
	}
	items := make([]model.Ticket, len(tickets))
	for i, t := range tickets {
		items[i] = *s.toModel(&t)
	}
	return &model.TicketListResp{Total: total, Items: items}, nil
}

func (s *TicketService) toModel(t *database.Ticket) *model.Ticket {
	return &model.Ticket{
		ID:          t.ID,
		Initiator:   t.Initiator,
		Category:    t.Category,
		Title:       t.Title,
		Content:     t.Content,
		Resolution:  t.Resolution,
		IsCompleted: t.IsCompleted,
		CreatedAt:   t.CreatedAt,
		CompletedAt: t.CompletedAt,
	}
}
