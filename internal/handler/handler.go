package handler

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"tix/internal/config"
	"tix/internal/database"
	"tix/internal/model"
	"tix/internal/service"
	"log"
	"net/http"
	"os"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Handler struct {
	svc        *service.TicketService
	categories []string
	cfg        *config.Config
}

func NewHandler(svc *service.TicketService, categories []string, cfg *config.Config) *Handler {
	return &Handler{svc: svc, categories: categories, cfg: cfg}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// 工单API
	mux.HandleFunc("POST /v1/tickets", h.CreateTicket)
	mux.HandleFunc("GET /v1/tickets", h.ListTickets)
	mux.HandleFunc("GET /v1/tickets/{id}", h.GetTicket)
	mux.HandleFunc("PATCH /v1/tickets/{id}", h.UpdateTicket)
	mux.HandleFunc("DELETE /v1/tickets/{id}", h.DeleteTicket)

	// 批量操作
	mux.HandleFunc("POST /v1/tickets/batch-delete", h.BatchDelete)
	mux.HandleFunc("POST /v1/tickets/batch-update", h.BatchUpdate)

	// 分类
	mux.HandleFunc("GET /v1/categories", h.GetCategories)

	// 导入导出
	mux.HandleFunc("GET /v1/export", h.ExportTickets)
	mux.HandleFunc("GET /v1/export/csv", h.ExportCSV)
	mux.HandleFunc("POST /v1/import", h.ImportTickets)

	// 报告
	mux.HandleFunc("GET /v1/report", h.ExportReport)

	// 统计
	mux.HandleFunc("GET /v1/stats", h.GetStats)
	mux.HandleFunc("GET /v1/initiators", h.GetInitiators)

	// 思源笔记
	mux.HandleFunc("POST /v1/push-siyuan", h.PushToSiyuan)

	// 配置
	mux.HandleFunc("GET /v1/config", h.GetConfig)
	mux.HandleFunc("GET /v1/config/ai", h.GetAIConfig)
	mux.HandleFunc("POST /v1/config/ai", h.SaveAIConfig)
	mux.HandleFunc("POST /v1/config/ai/test", h.TestAIConfig)
	mux.HandleFunc("GET /v1/config/pdf", h.GetPDFConfig)
	mux.HandleFunc("POST /v1/config/pdf", h.SavePDFConfig)
	mux.HandleFunc("GET /v1/config/siyuan", h.GetSiYuanConfig)
	mux.HandleFunc("POST /v1/config/siyuan", h.SaveSiYuanConfig)
	mux.HandleFunc("POST /v1/config/siyuan/test", h.TestSiYuanConfig)
	mux.HandleFunc("GET /v1/config/categories", h.GetCategoriesConfig)
	mux.HandleFunc("POST /v1/config/categories", h.AddCategory)
	mux.HandleFunc("PUT /v1/config/categories/{name}", h.UpdateCategory)
	mux.HandleFunc("DELETE /v1/config/categories/{name}", h.DeleteCategory)

	// 系统
	mux.HandleFunc("GET /v1/system/info", h.GetSystemInfo)
}

// ==================== 工单操作 ====================

func (h *Handler) CreateTicket(w http.ResponseWriter, r *http.Request) {
	var req model.CreateTicketReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.error(w, 400, "INVALID_JSON", "Invalid JSON body")
		return
	}

	req.Initiator = strings.TrimSpace(req.Initiator)
	req.Category = strings.TrimSpace(req.Category)
	req.Content = strings.TrimSpace(req.Content)

	// 验证
	if len(req.Initiator) < 1 || len(req.Initiator) > 50 {
		h.error(w, 400, "INVALID_INITIATOR", "initiator must be 1-50 characters")
		return
	}
	if req.Category != "" && !h.isValidCategory(req.Category) {
		h.error(w, 400, "INVALID_CATEGORY", "category not in allowed list")
		return
	}
	if len(req.Content) < 1 || len(req.Content) > 5000 {
		h.error(w, 400, "INVALID_CONTENT", "content must be 1-5000 characters")
		return
	}

	// 默认优先级
	if req.Priority < 1 || req.Priority > 4 {
		req.Priority = model.PriorityNormal
	}

	// 处理分类：AI自动选择
	if req.Category == "" {
		if len(h.cfg.Categories) == 0 {
			h.error(w, 400, "NO_CATEGORY", "请先在设置中创建分类")
			return
		}
		if h.cfg.AI.APIKey == "" {
			h.error(w, 400, "AI_NOT_CONFIGURED", "请先在设置中配置AI，或手动选择分类")
			return
		}
		// 动态创建 AI 服务，确保使用最新配置
		aiSvc := service.NewAIService(&h.cfg.AI)
		if aiSvc == nil {
			h.error(w, 500, "AI_ERROR", "AI服务初始化失败")
			return
		}
		// AI选择分类
		log.Printf("AI分类请求: content=%s, categories=%v", req.Content[:min(50, len(req.Content))], h.cfg.Categories)
		category, err := aiSvc.SelectCategoryFromList(req.Content, h.cfg.Categories)
		if err != nil {
			log.Printf("AI分类失败: %v", err)
			h.error(w, 500, "AI_ERROR", fmt.Sprintf("AI分类失败: %v，请手动选择分类", err))
			return
		}
		log.Printf("AI分类结果: %s", category)
		req.Category = category
	}

	t, err := h.svc.Create(&req)
	if err != nil {
		h.error(w, 500, "INTERNAL_ERROR", "Failed to create ticket")
		return
	}

	h.json(w, 201, t)
}

func (h *Handler) ListTickets(w http.ResponseWriter, r *http.Request) {
	opts := database.ListOptions{Limit: 20, Offset: 0}

	// 筛选条件
	if v := r.URL.Query().Get("completed"); v != "" {
		b := strings.ToLower(v) == "true"
		opts.Completed = &b
	}
	if v := r.URL.Query().Get("category"); v != "" {
		opts.Category = v
	}
	if v := r.URL.Query().Get("initiator"); v != "" {
		opts.Initiator = v
	}
	if v := r.URL.Query().Get("search"); v != "" {
		opts.Search = v
	}
	if v := r.URL.Query().Get("priority"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 4 {
			opts.Priority = n
		}
	}
	if v := r.URL.Query().Get("start_date"); v != "" {
		opts.StartDate = v
	}
	if v := r.URL.Query().Get("end_date"); v != "" {
		opts.EndDate = v
	}

	// 分页
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			opts.Limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			opts.Offset = n
		}
	}

	// 排序
	if v := r.URL.Query().Get("sort"); v != "" {
		opts.SortBy = v
	}
	if v := r.URL.Query().Get("order"); v != "" {
		opts.SortDesc = strings.ToLower(v) == "desc"
	}

	resp, err := h.svc.List(opts)
	if err != nil {
		h.error(w, 500, "INTERNAL_ERROR", "Failed to list tickets")
		return
	}
	h.json(w, 200, resp)
}

func (h *Handler) GetTicket(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, err := h.svc.Get(id)
	if err != nil {
		h.error(w, 404, "NOT_FOUND", "Ticket not found")
		return
	}
	h.json(w, 200, t)
}

func (h *Handler) UpdateTicket(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req model.UpdateTicketReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.error(w, 400, "INVALID_JSON", "Invalid JSON body")
		return
	}

	updates := make(map[string]any)
	if req.Category != nil {
		// 空分类不更新，非空分类需要验证
		if *req.Category != "" {
			if !h.isValidCategory(*req.Category) {
				h.error(w, 400, "INVALID_CATEGORY", "category not in allowed list")
				return
			}
			updates["category"] = *req.Category
		}
		// 空分类则不更新，保持原值
	}
	if req.Resolution != nil {
		updates["resolution"] = *req.Resolution
	}
	if req.IsCompleted != nil {
		updates["is_completed"] = *req.IsCompleted
		if *req.IsCompleted && req.CompletedAt == nil {
			now := time.Now().Format(time.RFC3339)
			updates["completed_at"] = now
		} else if !*req.IsCompleted {
			updates["completed_at"] = nil
		}
	}
	if req.CompletedAt != nil {
		updates["completed_at"] = *req.CompletedAt
	}
	if req.CreatedAt != nil {
		updates["created_at"] = *req.CreatedAt
	}
	if req.Priority != nil {
		if *req.Priority < 1 || *req.Priority > 4 {
			h.error(w, 400, "INVALID_PRIORITY", "priority must be 1-4")
			return
		}
		updates["priority"] = *req.Priority
	}
	if req.Tags != nil {
		updates["tags"] = *req.Tags
	}

	if len(updates) == 0 {
		h.error(w, 400, "NO_UPDATES", "No fields to update")
		return
	}

	if err := h.svc.Update(id, updates); err != nil {
		h.error(w, 500, "INTERNAL_ERROR", "Failed to update ticket")
		return
	}

	h.json(w, 200, map[string]string{"message": "updated"})
}

func (h *Handler) DeleteTicket(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.svc.Delete(id); err != nil {
		h.error(w, 500, "INTERNAL_ERROR", "Failed to delete ticket")
		return
	}
	h.json(w, 200, map[string]string{"message": "deleted"})
}

func (h *Handler) BatchDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.error(w, 400, "INVALID_JSON", "Invalid JSON body")
		return
	}
	if len(req.IDs) == 0 {
		h.error(w, 400, "NO_IDS", "No IDs provided")
		return
	}

	if err := h.svc.BatchDelete(req.IDs); err != nil {
		h.error(w, 500, "INTERNAL_ERROR", "Failed to delete tickets")
		return
	}
	h.json(w, 200, map[string]int{"deleted": len(req.IDs)})
}

func (h *Handler) BatchUpdate(w http.ResponseWriter, r *http.Request) {
	var req model.BatchUpdateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.error(w, 400, "INVALID_JSON", "Invalid JSON body")
		return
	}
	if len(req.IDs) == 0 {
		h.error(w, 400, "NO_IDS", "No IDs provided")
		return
	}
	if len(req.Updates) == 0 {
		h.error(w, 400, "NO_UPDATES", "No updates provided")
		return
	}

	// 验证更新字段
	if cat, ok := req.Updates["category"]; ok {
		if s, ok := cat.(string); !ok || !h.isValidCategory(s) {
			h.error(w, 400, "INVALID_CATEGORY", "invalid category")
			return
		}
	}
	if pri, ok := req.Updates["priority"]; ok {
		if n, ok := pri.(float64); !ok || n < 1 || n > 4 {
			h.error(w, 400, "INVALID_PRIORITY", "priority must be 1-4")
			return
		}
	}

	// 处理完成状态
	if isComp, ok := req.Updates["is_completed"]; ok {
		if b, ok := isComp.(bool); ok && b {
			req.Updates["completed_at"] = time.Now().Format(time.RFC3339)
		}
	}

	if err := h.svc.BatchUpdate(req.IDs, req.Updates); err != nil {
		h.error(w, 500, "INTERNAL_ERROR", "Failed to update tickets")
		return
	}
	h.json(w, 200, map[string]int{"updated": len(req.IDs)})
}

// ==================== 分类 ====================

func (h *Handler) GetCategories(w http.ResponseWriter, r *http.Request) {
	h.json(w, 200, h.categories)
}

// ==================== 统计 ====================

func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.svc.GetStats()
	if err != nil {
		h.error(w, 500, "INTERNAL_ERROR", "Failed to get stats")
		return
	}
	h.json(w, 200, stats)
}

func (h *Handler) GetInitiators(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	initiators, err := h.svc.GetInitiators(limit)
	if err != nil {
		h.error(w, 500, "INTERNAL_ERROR", "Failed to get initiators")
		return
	}
	h.json(w, 200, initiators)
}

// ==================== 导入导出 ====================

func (h *Handler) ExportTickets(w http.ResponseWriter, r *http.Request) {
	opts := database.ListOptions{Limit: 10000}
	tickets, _, err := h.svc.ListRaw(opts)
	if err != nil {
		h.error(w, 500, "INTERNAL_ERROR", "Failed to export tickets")
		return
	}

	// 导出时包含完整配置信息
	exportData := map[string]interface{}{
		"tickets": tickets,
		"config": map[string]interface{}{
			"categories": h.cfg.Categories,
			"ai": map[string]string{
				"api_key":  h.cfg.AI.APIKey,
				"base_url": h.cfg.AI.BaseURL,
				"model":    h.cfg.AI.Model,
			},
			"siyuan": map[string]string{
				"api_url":     h.cfg.SiYuan.APIURL,
				"notebook_id": h.cfg.SiYuan.NotebookID,
			},
			"pdf": map[string]string{
				"font_path": h.cfg.PDF.FontPath,
			},
		},
	}

	w.Header().Set("Content-Disposition", "attachment; filename=tickets_export.json")
	h.json(w, 200, exportData)
}

func (h *Handler) ExportCSV(w http.ResponseWriter, r *http.Request) {
	opts := database.ListOptions{Limit: 10000}
	tickets, _, err := h.svc.ListRaw(opts)
	if err != nil {
		h.error(w, 500, "INTERNAL_ERROR", "Failed to export tickets")
		return
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Header
	headers := []string{"ID", "发起人", "分类", "标题", "内容", "解决方案", "状态", "创建时间", "完成时间", "优先级", "标签"}
	writer.Write(headers)

	// 数据
	for _, t := range tickets {
		status := "处理中"
		if t.IsCompleted {
			status = "已完成"
		}
		completedAt := ""
		if t.CompletedAt != nil {
			completedAt = *t.CompletedAt
		}
		priorityNames := []string{"", "低", "普通", "高", "紧急"}
		row := []string{
			t.ID, t.Initiator, t.Category, t.Title, t.Content, t.Resolution,
			status, t.CreatedAt, completedAt, priorityNames[t.Priority], t.Tags,
		}
		writer.Write(row)
	}
	writer.Flush()

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=tickets_export.csv")
	// BOM for Excel
	w.Write([]byte{0xEF, 0xBB, 0xBF})
	w.Write(buf.Bytes())
}

func (h *Handler) ImportTickets(w http.ResponseWriter, r *http.Request) {
	// 解析 multipart form (最大 10MB)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		h.error(w, 400, "INVALID_FORM", "Failed to parse form")
		return
	}

	// 获取上传的文件
	file, _, err := r.FormFile("file")
	if err != nil {
		h.error(w, 400, "INVALID_FORM", "Missing file field")
		return
	}
	defer file.Close()

	// 解析 JSON - 支持两种格式
	var fileContent bytes.Buffer
	if _, err := fileContent.ReadFrom(file); err != nil {
		h.error(w, 500, "READ_ERROR", "Failed to read file")
		return
	}

	var tickets []model.Ticket
	configImported := false

	// 尝试解析新格式（包含 config）
	var exportData struct {
		Tickets []model.Ticket `json:"tickets"`
		Config  struct {
			Categories []string `json:"categories"`
			AI         struct {
				APIKey  string `json:"api_key"`
				BaseURL string `json:"base_url"`
				Model   string `json:"model"`
			} `json:"ai"`
			SiYuan struct {
				APIURL     string `json:"api_url"`
				NotebookID string `json:"notebook_id"`
			} `json:"siyuan"`
			PDF struct {
				FontPath string `json:"font_path"`
			} `json:"pdf"`
		} `json:"config"`
	}

	if err := json.Unmarshal(fileContent.Bytes(), &exportData); err == nil && len(exportData.Tickets) > 0 {
		// 新格式
		tickets = exportData.Tickets
		// 恢复配置
		if len(exportData.Config.Categories) > 0 {
			h.cfg.Categories = exportData.Config.Categories
			h.categories = h.cfg.Categories // 同步更新
			configImported = true
		}
		if exportData.Config.AI.APIKey != "" || exportData.Config.AI.Model != "" {
			h.cfg.AI.APIKey = exportData.Config.AI.APIKey
			h.cfg.AI.BaseURL = exportData.Config.AI.BaseURL
			h.cfg.AI.Model = exportData.Config.AI.Model
			configImported = true
		}
		if exportData.Config.SiYuan.NotebookID != "" {
			h.cfg.SiYuan.APIURL = exportData.Config.SiYuan.APIURL
			h.cfg.SiYuan.NotebookID = exportData.Config.SiYuan.NotebookID
			configImported = true
		}
		if exportData.Config.PDF.FontPath != "" {
			h.cfg.PDF.FontPath = exportData.Config.PDF.FontPath
			configImported = true
		}
		if configImported {
			config.Save(h.cfg)
		}
	} else {
		// 旧格式（只有工单数组）
		if err := json.Unmarshal(fileContent.Bytes(), &tickets); err != nil {
			h.error(w, 400, "INVALID_JSON", "Invalid JSON body")
			return
		}
	}

	imported, skipped, err := h.svc.Import(tickets)
	if err != nil {
		h.error(w, 500, "INTERNAL_ERROR", "Failed to import tickets")
		return
	}

	result := map[string]interface{}{
		"imported": imported,
		"skipped":  skipped,
	}
	if configImported {
		result["config_imported"] = true
	}

	h.json(w, 200, result)
}

// ==================== 配置 ====================

func (h *Handler) GetConfig(w http.ResponseWriter, r *http.Request) {
	cfg := map[string]any{
		"categories": h.cfg.Categories,
		"has_ai":     h.cfg.AI.APIKey != "",
		"has_siyuan": h.cfg.SiYuan.NotebookID != "",
		"pdf_font":   h.cfg.PDF.FontPath != "",
	}
	h.json(w, 200, cfg)
}

func (h *Handler) GetAIConfig(w http.ResponseWriter, r *http.Request) {
	h.json(w, 200, map[string]string{
		"api_key":  maskAPIKey(h.cfg.AI.APIKey),
		"base_url": h.cfg.AI.BaseURL,
		"model":    h.cfg.AI.Model,
	})
}

func (h *Handler) SaveAIConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		APIKey  *string `json:"api_key"`
		BaseURL *string `json:"base_url"`
		Model   *string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.error(w, 400, "INVALID_JSON", "Invalid JSON body")
		return
	}

	// 使用指针区分"未发送"和"发送空字符串"
	if req.APIKey != nil {
		// 如果发送的是空字符串或 "CLEAR"，则清空配置
		if *req.APIKey == "" || *req.APIKey == "CLEAR" {
			h.cfg.AI.APIKey = ""
		} else if !strings.Contains(*req.APIKey, "****") {
			// 脱敏格式不更新
			h.cfg.AI.APIKey = *req.APIKey
		}
	}
	if req.BaseURL != nil {
		h.cfg.AI.BaseURL = *req.BaseURL
	}
	if req.Model != nil {
		h.cfg.AI.Model = *req.Model
	}

	if err := h.saveConfig(); err != nil {
		h.error(w, 500, "SAVE_ERROR", "Failed to save config")
		return
	}
	h.json(w, 200, map[string]string{"message": "saved"})
}

func (h *Handler) TestAIConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		APIKey  string `json:"api_key"`
		BaseURL string `json:"base_url"`
		Model   string `json:"model"`
	}
	// 允许空 body
	json.NewDecoder(r.Body).Decode(&req)

	// 使用实际传入值或配置值
	apiKey := req.APIKey
	if apiKey == "" {
		apiKey = h.cfg.AI.APIKey
	}
	baseURL := req.BaseURL
	if baseURL == "" {
		baseURL = h.cfg.AI.BaseURL
	}
	model := req.Model
	if model == "" {
		model = h.cfg.AI.Model
	}

	if apiKey == "" {
		h.error(w, 400, "NO_API_KEY", "API key is required")
		return
	}

	result, err := h.svc.TestAI(apiKey, baseURL, model)
	if err != nil {
		h.error(w, 500, "TEST_FAILED", err.Error())
		return
	}
	h.json(w, 200, map[string]string{"result": result})
}

func (h *Handler) GetPDFConfig(w http.ResponseWriter, r *http.Request) {
	fontPath := h.cfg.PDF.FontPath
	if fontPath == "" {
		fontPath = config.DetectFontPath()
	}
	h.json(w, 200, map[string]string{
		"font_path":       fontPath,
		"current_os":      runtime.GOOS,
		"detected_path":   config.DetectFontPath(),
	})
}

func (h *Handler) SavePDFConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FontPath string `json:"font_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.error(w, 400, "INVALID_JSON", "Invalid JSON body")
		return
	}

	h.cfg.PDF.FontPath = req.FontPath

	if err := h.saveConfig(); err != nil {
		h.error(w, 500, "SAVE_ERROR", "Failed to save config")
		return
	}
	h.json(w, 200, map[string]string{"message": "saved"})
}

func (h *Handler) GetSiYuanConfig(w http.ResponseWriter, r *http.Request) {
	h.json(w, 200, map[string]string{
		"api_url":     h.cfg.SiYuan.APIURL,
		"notebook_id": h.cfg.SiYuan.NotebookID,
	})
}

func (h *Handler) SaveSiYuanConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		APIURL     string `json:"api_url"`
		NotebookID string `json:"notebook_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.error(w, 400, "INVALID_JSON", "Invalid JSON body")
		return
	}

	h.cfg.SiYuan.APIURL = req.APIURL
	h.cfg.SiYuan.NotebookID = req.NotebookID

	if err := h.saveConfig(); err != nil {
		h.error(w, 500, "SAVE_ERROR", "Failed to save config")
		return
	}
	h.json(w, 200, map[string]string{"message": "saved"})
}

func (h *Handler) TestSiYuanConfig(w http.ResponseWriter, r *http.Request) {
	apiURL := h.cfg.SiYuan.APIURL
	if apiURL == "" {
		apiURL = "http://127.0.0.1:6806"
	}

	resp, err := http.Get(apiURL + "/api/system/version")
	if err != nil {
		h.error(w, 500, "CONNECTION_FAILED", err.Error())
		return
	}
	defer resp.Body.Close()

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data string `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if result.Code != 0 {
		h.error(w, 500, "API_ERROR", result.Msg)
		return
	}

	h.json(w, 200, map[string]string{"version": result.Data})
}

func (h *Handler) GetCategoriesConfig(w http.ResponseWriter, r *http.Request) {
	h.json(w, 200, h.cfg.Categories)
}

func (h *Handler) AddCategory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.error(w, 400, "INVALID_JSON", "Invalid JSON body")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		h.error(w, 400, "EMPTY_NAME", "Category name is required")
		return
	}

	if slices.Contains(h.cfg.Categories, req.Name) {
		h.error(w, 400, "DUPLICATE", "Category already exists")
		return
	}

	h.cfg.Categories = append(h.cfg.Categories, req.Name)
	sort.Strings(h.cfg.Categories)
	h.categories = h.cfg.Categories

	if err := h.saveConfig(); err != nil {
		h.error(w, 500, "SAVE_ERROR", "Failed to save config")
		return
	}
	h.json(w, 201, h.cfg.Categories)
}

func (h *Handler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	oldName := r.PathValue("name")
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.error(w, 400, "INVALID_JSON", "Invalid JSON body")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		h.error(w, 400, "EMPTY_NAME", "Category name is required")
		return
	}

	idx := slices.Index(h.cfg.Categories, oldName)
	if idx == -1 {
		h.error(w, 404, "NOT_FOUND", "Category not found")
		return
	}

	if slices.Contains(h.cfg.Categories, req.Name) && req.Name != oldName {
		h.error(w, 400, "DUPLICATE", "Category already exists")
		return
	}

	h.cfg.Categories[idx] = req.Name
	h.categories = h.cfg.Categories

	if err := h.saveConfig(); err != nil {
		h.error(w, 500, "SAVE_ERROR", "Failed to save config")
		return
	}
	h.json(w, 200, h.cfg.Categories)
}

func (h *Handler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	idx := slices.Index(h.cfg.Categories, name)
	if idx == -1 {
		h.error(w, 404, "NOT_FOUND", "Category not found")
		return
	}

	h.cfg.Categories = slices.Delete(h.cfg.Categories, idx, idx+1)
	h.categories = h.cfg.Categories

	if err := h.saveConfig(); err != nil {
		h.error(w, 500, "SAVE_ERROR", "Failed to save config")
		return
	}
	h.json(w, 200, h.cfg.Categories)
}

// ==================== 系统信息 ====================

func (h *Handler) GetSystemInfo(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	info := map[string]any{
		"version":      "1.0.0",
		"go_version":   runtime.Version(),
		"os":           runtime.GOOS,
		"arch":         runtime.GOARCH,
		"cpu_count":    runtime.NumCPU(),
		"goroutines":   runtime.NumGoroutine(),
		"memory_mb":    m.Alloc / 1024 / 1024,
		"heap_mb":      m.HeapAlloc / 1024 / 1024,
	}
	h.json(w, 200, info)
}

// ==================== 思源笔记 ====================

func (h *Handler) PushToSiyuan(w http.ResponseWriter, r *http.Request) {
	startDate := r.URL.Query().Get("start_date")
	endDate := r.URL.Query().Get("end_date")

	if h.cfg.SiYuan.APIURL == "" || h.cfg.SiYuan.NotebookID == "" {
		h.error(w, 400, "NO_CONFIG", "SiYuan not configured")
		return
	}

	opts := database.ListOptions{
		StartDate: startDate,
		EndDate:   endDate,
		Limit:     10000,
	}
	tickets, _, err := h.svc.ListRaw(opts)
	if err != nil {
		h.error(w, 500, "INTERNAL_ERROR", "Failed to get tickets")
		return
	}

	if len(tickets) == 0 {
		h.json(w, 200, map[string]string{"message": "no tickets to push"})
		return
	}

	// 按月分组
	byMonth := make(map[string][]model.Ticket)
	for _, t := range tickets {
		month := t.CreatedAt[:7] // YYYY-MM
		byMonth[month] = append(byMonth[month], t)
	}

	pushed := 0
	for month, ts := range byMonth {
		docTitle := fmt.Sprintf("%s月工作报告", formatMonth(month))
		content := buildSiyuanContent(ts, docTitle)

		if err := pushToSiyuanNote(h.cfg.SiYuan.APIURL, h.cfg.SiYuan.NotebookID, docTitle, content); err != nil {
			log.Printf("Failed to push to SiYuan for %s: %v", month, err)
			continue
		}
		pushed += len(ts)
	}

	h.json(w, 200, map[string]int{
		"pushed": pushed,
		"months": len(byMonth),
	})
}

// ==================== 工具函数 ====================

func (h *Handler) isValidCategory(cat string) bool {
	return slices.Contains(h.categories, cat)
}

func (h *Handler) saveConfig() error {
	data, err := yaml.Marshal(h.cfg)
	if err != nil {
		return err
	}
	return os.WriteFile("config.yaml", data, 0644)
}

func (h *Handler) json(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *Handler) error(w http.ResponseWriter, status int, code, message string) {
	h.json(w, status, model.ErrorResp{
		Error: model.ErrorDetail{
			Code:    code,
			Message: message,
		},
	})
}

func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "********"
	}
	return key[:4] + "****" + key[len(key)-4:]
}

func formatMonth(month string) string {
	// "2026-03" -> "2026年3月"
	parts := strings.Split(month, "-")
	if len(parts) != 2 {
		return month
	}
	year := parts[0]
	m := parts[1]
	if strings.HasPrefix(m, "0") {
		m = m[1:]
	}
	return fmt.Sprintf("%s年%s", year, m)
}

func buildSiyuanContent(tickets []model.Ticket, title string) string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("# %s\n\n", title))

	// 按日期分组
	byDate := make(map[string][]model.Ticket)
	for _, t := range tickets {
		date := t.CreatedAt[:10]
		byDate[date] = append(byDate[date], t)
	}

	// 排序日期
	dates := make([]string, 0, len(byDate))
	for d := range byDate {
		dates = append(dates, d)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dates)))

	for _, date := range dates {
		buf.WriteString(fmt.Sprintf("## %s\n\n", date))
		for _, t := range byDate[date] {
			status := "✅"
			if !t.IsCompleted {
				status = "⏳"
			}
			buf.WriteString(fmt.Sprintf("- %s **%s** - %s\n", status, t.Initiator, t.Title))
			if t.Content != "" {
				buf.WriteString(fmt.Sprintf("  - 内容: %s\n", t.Content))
			}
			if t.Resolution != "" {
				buf.WriteString(fmt.Sprintf("  - 解决: %s\n", t.Resolution))
			}
		}
		buf.WriteString("\n")
	}

	return buf.String()
}

func pushToSiyuanNote(apiURL, notebookID, title, content string) error {
	payload := map[string]any{
		"notebook": notebookID,
		"path":     "/" + title,
		"markdown": content,
	}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(apiURL+"/api/filetree/createDocWithMd", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Code int `json:"code"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if result.Code != 0 {
		return fmt.Errorf("SiYuan API error: code %d", result.Code)
	}

	return nil
}
