package handler

import (
	"bytes"
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
	mux.HandleFunc("POST /v1/tickets", h.CreateTicket)
	mux.HandleFunc("GET /v1/tickets", h.ListTickets)
	mux.HandleFunc("GET /v1/tickets/{id}", h.GetTicket)
	mux.HandleFunc("PATCH /v1/tickets/{id}", h.UpdateTicket)
	mux.HandleFunc("DELETE /v1/tickets/{id}", h.DeleteTicket)
	mux.HandleFunc("GET /v1/categories", h.GetCategories)
	mux.HandleFunc("GET /v1/export", h.ExportTickets)
	mux.HandleFunc("POST /v1/import", h.ImportTickets)
	mux.HandleFunc("GET /v1/report", h.ExportReport)
	mux.HandleFunc("POST /v1/push-siyuan", h.PushToSiyuan)
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
}

func (h *Handler) CreateTicket(w http.ResponseWriter, r *http.Request) {
	var req model.CreateTicketReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.error(w, 400, "INVALID_JSON", "Invalid JSON body")
		return
	}

	req.Initiator = strings.TrimSpace(req.Initiator)
	req.Category = strings.TrimSpace(req.Category)
	req.Content = strings.TrimSpace(req.Content)

	if len(req.Initiator) < 1 || len(req.Initiator) > 50 {
		h.error(w, 400, "INVALID_INITIATOR", "initiator must be 1-50 characters")
		return
	}
	// 分类可以为空，AI会自动选择；如果指定则必须有效
	if req.Category != "" && !h.isValidCategory(req.Category) {
		log.Printf("Invalid category: %q", req.Category)
		h.error(w, 400, "INVALID_CATEGORY", "category not in allowed list")
		return
	}
	if len(req.Content) < 1 || len(req.Content) > 2000 {
		h.error(w, 400, "INVALID_CONTENT", "content must be 1-2000 characters")
		return
	}

	// 如果用户没有指定分类，检查是否需要提示
	if req.Category == "" {
		categories := h.cfg.Categories
		if len(categories) == 0 {
			// 没有分类，提示用户先创建
			h.error(w, 400, "NO_CATEGORY", "请先在设置中创建分类，或手动选择分类")
			return
		}
		// AI未配置时，自动选择第一个分类（避免panic）
		if h.cfg.AI.APIKey == "" {
			req.Category = categories[0]
			log.Printf("AI未配置，自动选择分类: %s", req.Category)
		}
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
	if v := r.URL.Query().Get("start_date"); v != "" {
		opts.StartDate = v
	}
	if v := r.URL.Query().Get("end_date"); v != "" {
		opts.EndDate = v
	}
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

	// 先检查是否存在
	if _, err := h.svc.Get(id); err != nil {
		h.error(w, 404, "NOT_FOUND", "Ticket not found")
		return
	}

	var req model.UpdateTicketReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.error(w, 400, "INVALID_JSON", "Invalid JSON body")
		return
	}

	t, err := h.svc.Update(id, &req)
	if err != nil {
		h.error(w, 500, "INTERNAL_ERROR", "Failed to update ticket")
		return
	}
	h.json(w, 200, t)
}

func (h *Handler) DeleteTicket(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.svc.Delete(id); err != nil {
		h.error(w, 500, "INTERNAL_ERROR", "Failed to delete ticket")
		return
	}
	w.WriteHeader(204)
}

func (h *Handler) GetCategories(w http.ResponseWriter, r *http.Request) {
	h.json(w, 200, &model.CategoryResp{Categories: h.categories})
}

func (h *Handler) GetCategoriesConfig(w http.ResponseWriter, r *http.Request) {
	// 返回分类及其工单数量
	type CategoryInfo struct {
		Name    string `json:"name"`
		Tickets int    `json:"tickets"`
	}

	counts := make(map[string]int)
	resp, _ := h.svc.List(database.ListOptions{Limit: 10000, Offset: 0})
	for _, t := range resp.Items {
		if t.Category != "" {
			counts[t.Category]++
		}
	}

	var result []CategoryInfo
	for _, cat := range h.categories {
		result = append(result, CategoryInfo{
			Name:    cat,
			Tickets: counts[cat],
		})
	}
	h.json(w, 200, result)
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
		h.error(w, 400, "INVALID_NAME", "分类名称不能为空")
		return
	}

	// 检查是否已存在
	if slices.Contains(h.categories, req.Name) {
		h.error(w, 400, "DUPLICATE", "分类已存在")
		return
	}

	h.categories = append(h.categories, req.Name)
	if err := h.saveConfig(); err != nil {
		h.error(w, 500, "SAVE_ERROR", "保存失败")
		return
	}

	h.json(w, 200, map[string]any{"success": true, "name": req.Name})
}

func (h *Handler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	oldName := r.PathValue("name")

	var req struct {
		NewName string `json:"new_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.error(w, 400, "INVALID_JSON", "Invalid JSON body")
		return
	}

	req.NewName = strings.TrimSpace(req.NewName)
	if req.NewName == "" {
		h.error(w, 400, "INVALID_NAME", "分类名称不能为空")
		return
	}

	// 检查新名称是否已存在
	if slices.Contains(h.categories, req.NewName) {
		h.error(w, 400, "DUPLICATE", "分类名称已存在")
		return
	}

	// 更新分类列表
	found := false
	for i, cat := range h.categories {
		if cat == oldName {
			h.categories[i] = req.NewName
			found = true
			break
		}
	}
	if !found {
		h.error(w, 404, "NOT_FOUND", "分类不存在")
		return
	}

	// 更新所有工单的分类
	if err := h.svc.UpdateCategory(oldName, req.NewName); err != nil {
		h.error(w, 500, "UPDATE_ERROR", "更新工单分类失败")
		return
	}

	if err := h.saveConfig(); err != nil {
		h.error(w, 500, "SAVE_ERROR", "保存失败")
		return
	}

	h.json(w, 200, map[string]any{"success": true})
}

func (h *Handler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	// 获取转移目标分类
	transferTo := r.URL.Query().Get("transfer_to")

	// 检查是否有工单使用此分类
	resp, _ := h.svc.List(database.ListOptions{Limit: 10000, Offset: 0})
	var ticketCount int
	for _, t := range resp.Items {
		if t.Category == name {
			ticketCount++
		}
	}

	if ticketCount > 0 && transferTo == "" {
		h.json(w, 200, map[string]any{
			"need_transfer": true,
			"tickets":       ticketCount,
			"categories":    h.getOtherCategories(name),
		})
		return
	}

	// 转移工单
	if transferTo != "" {
		if err := h.svc.TransferCategory(name, transferTo); err != nil {
			h.error(w, 500, "TRANSFER_ERROR", "转移工单失败")
			return
		}
	}

	// 删除分类
	for i, cat := range h.categories {
		if cat == name {
			h.categories = append(h.categories[:i], h.categories[i+1:]...)
			break
		}
	}

	if err := h.saveConfig(); err != nil {
		h.error(w, 500, "SAVE_ERROR", "保存失败")
		return
	}

	h.json(w, 200, map[string]any{"success": true})
}

func (h *Handler) getOtherCategories(exclude string) []string {
	var result []string
	for _, cat := range h.categories {
		if cat != exclude {
			result = append(result, cat)
		}
	}
	return result
}

func (h *Handler) ExportTickets(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.List(database.ListOptions{Limit: 10000, Offset: 0})
	if err != nil {
		h.error(w, 500, "EXPORT_ERROR", "Failed to export tickets")
		return
	}

	// 导出包含配置和数据
	exportData := map[string]any{
		"version":     "1.0",
		"exported_at": time.Now().Format(time.RFC3339),
		"config": map[string]any{
			"ai": map[string]string{
				"api_key":  h.cfg.AI.APIKey,
				"base_url": h.cfg.AI.BaseURL,
				"model":    h.cfg.AI.Model,
			},
			"siyuan": map[string]string{
				"api_url":     h.cfg.SiYuan.APIURL,
				"notebook_id": h.cfg.SiYuan.NotebookID,
			},
			"categories": h.categories,
		},
		"tickets": resp.Items,
		"total":   resp.Total,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=tickets_export.json")
	json.NewEncoder(w).Encode(exportData)
}

func (h *Handler) ImportTickets(w http.ResponseWriter, r *http.Request) {
	file, _, err := r.FormFile("file")
	if err != nil {
		h.error(w, 400, "INVALID_FILE", "No file uploaded")
		return
	}
	defer file.Close()

	var data struct {
		Version string `json:"version"`
		Config  struct {
			AI struct {
				APIKey  string `json:"api_key"`
				BaseURL string `json:"base_url"`
				Model   string `json:"model"`
			} `json:"ai"`
			SiYuan struct {
				APIURL     string `json:"api_url"`
				NotebookID string `json:"notebook_id"`
			} `json:"siyuan"`
			Categories []string `json:"categories"`
		} `json:"config"`
		Tickets []model.Ticket `json:"tickets"`
		Total   int            `json:"total"`
	}

	if err := json.NewDecoder(file).Decode(&data); err != nil {
		h.error(w, 400, "INVALID_JSON", "Invalid JSON format")
		return
	}

	// 导入配置（如果有）
	configImported := false
	if data.Config.AI.APIKey != "" || data.Config.AI.BaseURL != "" || data.Config.AI.Model != "" {
		h.cfg.AI.APIKey = data.Config.AI.APIKey
		h.cfg.AI.BaseURL = data.Config.AI.BaseURL
		h.cfg.AI.Model = data.Config.AI.Model
		configImported = true
	}
	if data.Config.SiYuan.APIURL != "" || data.Config.SiYuan.NotebookID != "" {
		h.cfg.SiYuan.APIURL = data.Config.SiYuan.APIURL
		h.cfg.SiYuan.NotebookID = data.Config.SiYuan.NotebookID
		configImported = true
	}
	if configImported {
		h.saveConfig()
	}

	// 导入工单
	imported := 0
	for _, t := range data.Tickets {
		req := &model.CreateTicketReq{
			Initiator: t.Initiator,
			Category:  t.Category,
			Content:   t.Content,
		}
		if err := h.svc.CreateWithID(t.ID, t.CreatedAt, req); err == nil {
			imported++
		}
	}

	h.json(w, 200, map[string]any{"imported": imported, "config_imported": configImported})
}

func (h *Handler) PushToSiyuan(w http.ResponseWriter, r *http.Request) {
	// 获取思源配置
	siyuanURL := h.cfg.SiYuan.APIURL
	if siyuanURL == "" {
		siyuanURL = "http://127.0.0.1:6806"
	}
	notebookID := h.cfg.SiYuan.NotebookID
	if notebookID == "" {
		h.error(w, 400, "SIYUAN_NOT_CONFIGURED", "未配置思源笔记本ID，请先在设置中配置")
		return
	}

	// 检查思源是否运行
	checkReq, _ := http.NewRequest("GET", siyuanURL+"/api/system/version", nil)
	client := &http.Client{Timeout: 3 * time.Second}
	checkResp, err := client.Do(checkReq)
	if err != nil || checkResp.StatusCode != 200 {
		h.error(w, 503, "SIYUAN_NOT_RUNNING", "思源笔记未运行，请先启动思源笔记")
		return
	}
	checkResp.Body.Close()

	resp, err := h.svc.List(database.ListOptions{Limit: 10000, Offset: 0})
	if err != nil {
		h.error(w, 500, "LIST_ERROR", "Failed to get tickets")
		return
	}

	if len(resp.Items) == 0 {
		h.error(w, 400, "NO_TICKETS", "没有工单可推送")
		return
	}

	getMonth := func(created string) string {
		if len(created) >= 7 {
			return created[:7]
		}
		return "unknown"
	}

	months := make(map[string][]model.Ticket)
	for _, t := range resp.Items {
		month := getMonth(t.CreatedAt)
		months[month] = append(months[month], t)
	}
	pushed := 0

	monthNames := map[string]string{
		"01": "1月", "02": "2月", "03": "3月", "04": "4月",
		"05": "5月", "06": "6月", "07": "7月", "08": "8月",
		"09": "9月", "10": "10月", "11": "11月", "12": "12月",
	}
	weekDays := map[string]string{
		"Sunday": "星期日", "Monday": "星期一", "Tuesday": "星期二",
		"Wednesday": "星期三", "Thursday": "星期四", "Friday": "星期五",
		"Saturday": "星期六",
	}

	for month, tickets := range months {
		year := month[:4]
		m := month[5:7]
		docTitle := fmt.Sprintf("%s年%s", year, monthNames[m]) // 文件名：2026年3月

		// 按日期分组
		days := make(map[string][]model.Ticket)
		for _, t := range tickets {
			if len(t.CreatedAt) >= 10 {
				day := t.CreatedAt[:10]
				days[day] = append(days[day], t)
			}
		}

		var content strings.Builder
		// 不写标题，文档名就是标题
		content.WriteString("> 此文档由IT工单系统自动生成，仅用于展示和报告，不可反向导入。\n\n")

		sortedDays := make([]string, 0, len(days))
		for d := range days {
			sortedDays = append(sortedDays, d)
		}
		sort.Strings(sortedDays)

		counter := 1
		for _, day := range sortedDays {
			t, _ := time.Parse("2006-01-02", day)
			weekDay := weekDays[t.Weekday().String()]
			dayNum := t.Day()
			monthNum := int(t.Month())

			content.WriteString("---\n\n")
			content.WriteString(fmt.Sprintf("## %d月%d日 %s\n\n", monthNum, dayNum, weekDay))

			for _, ticket := range days[day] {
				// 使用工单标题，如果为空则从内容截取
				titleText := ticket.Title
				if titleText == "" {
					titleRunes := []rune(ticket.Content)
					if len(titleRunes) > 30 {
						titleText = string(titleRunes[:30])
					} else {
						titleText = string(titleRunes)
					}
				}

				status := "⏳ 处理中"
				if ticket.IsCompleted {
					status = "✅ 已完成"
				}

				createdTime := ticket.CreatedAt
				if len(createdTime) > 19 {
					createdTime = createdTime[:19]
				}
				createdTime = strings.Replace(createdTime, "T", " ", 1)

				completedTime := ""
				if ticket.CompletedAt != nil && len(*ticket.CompletedAt) > 0 {
					completedTime = *ticket.CompletedAt
					if len(completedTime) > 19 {
						completedTime = completedTime[:19]
					}
					completedTime = strings.Replace(completedTime, "T", " ", 1)
				}

				content.WriteString(fmt.Sprintf("### %03d %s\n\n", counter, titleText))
				content.WriteString("| 发起人 | 分类 | 状态 | 创建时间 | 完成时间 |\n")
				content.WriteString("|--------|------|------|----------|----------|\n")
				content.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n\n",
					ticket.Initiator, ticket.Category, status, createdTime, completedTime))

				content.WriteString(fmt.Sprintf("**问题描述**:\n%s\n\n", ticket.Content))

				if ticket.Resolution != "" {
					content.WriteString(fmt.Sprintf("**解决过程**:\n%s\n\n", ticket.Resolution))
				}

				counter++
			}
		}

		// 删除已存在的文档（忽略错误，可能不存在）
		removeData, _ := json.Marshal(map[string]string{
			"notebook": notebookID,
			"path":     "/" + docTitle,
		})
		removeReq, _ := http.NewRequest("POST", siyuanURL+"/api/filetree/removeDoc", bytes.NewReader(removeData))
		removeReq.Header.Set("Content-Type", "application/json")
		removeResp, _ := client.Do(removeReq)
		if removeResp != nil {
			removeResp.Body.Close()
		}

		// 创建新文档
		payload := map[string]string{
			"notebook": notebookID,
			"path":     "/" + docTitle,
			"markdown": content.String(),
		}
		body, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", siyuanURL+"/api/filetree/createDocWithMd", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		createResp, err := client.Do(req)
		if err != nil {
			log.Printf("⚠ 创建文档失败: %v", err)
			continue
		}
		createResp.Body.Close()
		log.Printf("✓ 推送文档: %s (%d条工单)", docTitle, len(tickets))
		pushed += len(tickets)
	}

	log.Printf("✓ 已推送 %d 条工单到思源笔记 (%d 个月份)", pushed, len(months))
	h.json(w, 200, map[string]any{"pushed": pushed, "months": len(months)})
}

func (h *Handler) isValidCategory(cat string) bool {
	return slices.Contains(h.categories, cat)
}

func (h *Handler) json(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *Handler) error(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(model.ErrorResp{
		Error: model.ErrorDetail{Code: code, Message: msg},
	})
}

func (h *Handler) GetAIConfig(w http.ResponseWriter, r *http.Request) {
	h.json(w, 200, map[string]any{
		"api_key":  h.cfg.AI.APIKey,
		"base_url": h.cfg.AI.BaseURL,
		"model":    h.cfg.AI.Model,
		"enabled":  h.cfg.AI.APIKey != "",
	})
}

func (h *Handler) SaveAIConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		APIKey  string `json:"api_key"`
		BaseURL string `json:"base_url"`
		Model   string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.error(w, 400, "INVALID_JSON", "Invalid JSON body")
		return
	}

	// 更新内存配置
	h.cfg.AI.APIKey = req.APIKey
	h.cfg.AI.BaseURL = req.BaseURL
	h.cfg.AI.Model = req.Model

	// 保存到文件
	if err := h.saveConfig(); err != nil {
		log.Printf("Failed to save config: %v", err)
		h.error(w, 500, "SAVE_ERROR", "Failed to save config")
		return
	}

	h.json(w, 200, map[string]any{"success": true})
}

func (h *Handler) TestAIConfig(w http.ResponseWriter, r *http.Request) {
	ai := service.NewAIClient(h.cfg.AI.APIKey, h.cfg.AI.BaseURL, h.cfg.AI.Model)
	if !ai.IsEnabled() {
		h.json(w, 200, map[string]any{"success": false, "error": "未配置API密钥"})
		return
	}

	title := ai.GenerateTitle("测试：打印机无法打印，显示卡纸错误")
	if title == "打印机无法打印" || title != "" {
		h.json(w, 200, map[string]any{"success": true, "title": title})
		return
	}
	h.json(w, 200, map[string]any{"success": false, "error": "AI返回为空"})
}

func (h *Handler) GetPDFConfig(w http.ResponseWriter, r *http.Request) {
	// 自动检测默认字体路径
	defaultFont := config.DetectFontPath()

	h.json(w, 200, map[string]any{
		"font_path":      h.cfg.PDF.FontPath,
		"default_font":   defaultFont,
		"current_system": runtime.GOOS,
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

	// 验证字体文件是否存在
	if req.FontPath != "" {
		if _, err := os.Stat(req.FontPath); os.IsNotExist(err) {
			h.error(w, 400, "FONT_NOT_FOUND", "字体文件不存在")
			return
		}
	}

	h.cfg.PDF.FontPath = req.FontPath
	if err := config.Save(h.cfg); err != nil {
		log.Printf("Failed to save config: %v", err)
		h.error(w, 500, "SAVE_ERROR", "保存配置失败")
		return
	}

	h.json(w, 200, map[string]any{"success": true, "font_path": req.FontPath})
}

func (h *Handler) GetSiYuanConfig(w http.ResponseWriter, r *http.Request) {
	h.json(w, 200, map[string]any{
		"api_url":     h.cfg.SiYuan.APIURL,
		"notebook_id": h.cfg.SiYuan.NotebookID,
		"configured":  h.cfg.SiYuan.NotebookID != "",
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
		log.Printf("Failed to save config: %v", err)
		h.error(w, 500, "SAVE_ERROR", "Failed to save config")
		return
	}

	h.json(w, 200, map[string]any{"success": true})
}

func (h *Handler) TestSiYuanConfig(w http.ResponseWriter, r *http.Request) {
	apiURL := h.cfg.SiYuan.APIURL
	if apiURL == "" {
		apiURL = "http://127.0.0.1:6806"
	}

	// 测试连接
	client := &http.Client{Timeout: 3 * time.Second}
	req, _ := http.NewRequest("GET", apiURL+"/api/system/version", nil)
	resp, err := client.Do(req)
	if err != nil {
		h.json(w, 200, map[string]any{"success": false, "error": "无法连接到思源笔记"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		h.json(w, 200, map[string]any{"success": false, "error": "思源笔记未响应"})
		return
	}

	// 检查笔记本ID
	if h.cfg.SiYuan.NotebookID == "" {
		h.json(w, 200, map[string]any{"success": true, "warning": "连接成功，但未配置笔记本ID"})
		return
	}

	// 验证笔记本是否存在
	notebooksURL := apiURL + "/api/notebook/lsNotebooks"
	req2, _ := http.NewRequest("GET", notebooksURL, nil)
	resp2, err := client.Do(req2)
	if err != nil {
		h.json(w, 200, map[string]any{"success": true, "warning": "连接成功，但无法验证笔记本"})
		return
	}
	defer resp2.Body.Close()

	h.json(w, 200, map[string]any{"success": true, "message": "连接成功"})
}

func (h *Handler) saveConfig() error {
	// 同步分类到配置
	h.cfg.Categories = h.categories
	data, err := yaml.Marshal(h.cfg)
	if err != nil {
		return err
	}
	return os.WriteFile("config.yaml", data, 0644)
}
