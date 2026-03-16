package handler

import (
	"log"
	"net/http"
	"regexp"
	"strings"
	"tix/internal/model"
)

var publicPhonePattern = regexp.MustCompile(`^1\d{10}$`)

func normalizePhone(phone string) string {
	replacer := strings.NewReplacer(" ", "", "-", "", "+86", "")
	return replacer.Replace(strings.TrimSpace(phone))
}

func isValidPublicPhone(phone string) bool {
	return publicPhonePattern.MatchString(normalizePhone(phone))
}

func (h *Handler) SendPublicSMSCode(w http.ResponseWriter, r *http.Request) {
	phone := normalizePhone(r.URL.Query().Get("phone"))
	if !isValidPublicPhone(phone) {
		h.error(w, 400, "INVALID_PHONE", "请输入正确的 11 位手机号")
		return
	}

	const code = "123456"
	h.smsMu.Lock()
	h.smsCodes[phone] = code
	h.smsMu.Unlock()

	log.Printf("public sms code generated: phone=%s code=%s", phone, code)
	h.json(w, 200, map[string]any{
		"message": "验证码已发送（测试模式）",
		"code":    code,
	})
}

func (h *Handler) CreatePublicTicket(w http.ResponseWriter, r *http.Request) {
	var req model.CreatePublicTicketReq
	if err := h.decodeJSON(r, &req); err != nil {
		h.error(w, 400, "INVALID_JSON", "Invalid JSON body")
		return
	}

	req.Category = strings.TrimSpace(req.Category)
	req.Content = strings.TrimSpace(req.Content)
	req.Contact = strings.TrimSpace(req.Contact)
	req.Phone = normalizePhone(req.Phone)
	req.SMSCode = strings.TrimSpace(req.SMSCode)

	if req.Category == "" {
		h.error(w, 400, "INVALID_CATEGORY", "category is required")
		return
	}
	if !h.isValidCategory(req.Category) {
		h.error(w, 400, "INVALID_CATEGORY", "category not in allowed list")
		return
	}
	if len(req.Content) < 1 || len(req.Content) > 5000 {
		h.error(w, 400, "INVALID_CONTENT", "content must be 1-5000 characters")
		return
	}
	if len(req.Contact) > 50 {
		h.error(w, 400, "INVALID_CONTACT", "contact must be 0-50 characters")
		return
	}
	if !isValidPublicPhone(req.Phone) {
		h.error(w, 400, "INVALID_PHONE", "请输入正确的 11 位手机号")
		return
	}
	if req.SMSCode == "" {
		h.error(w, 400, "INVALID_SMS_CODE", "请输入短信验证码")
		return
	}

	h.smsMu.RLock()
	sentCode := h.smsCodes[req.Phone]
	h.smsMu.RUnlock()
	if req.SMSCode != "123456" && req.SMSCode != sentCode {
		h.error(w, 400, "INVALID_SMS_CODE", "短信验证码错误")
		return
	}

	initiator := "匿名提交"
	if req.Contact != "" {
		initiator = req.Contact
	}

	ticket, err := h.svc.Create(r.Context(), &model.CreateTicketReq{
		Initiator: initiator,
		Category:  req.Category,
		Content:   req.Content,
		Priority:  model.PriorityNormal,
		Tags:      "public",
	})
	if err != nil {
		h.error(w, 500, "INTERNAL_ERROR", "Failed to create ticket")
		return
	}

	h.json(w, 201, model.PublicTicketResp{
		ID:         ticket.ID,
		Category:   ticket.Category,
		CreatedAt:  ticket.CreatedAt,
		Title:      ticket.Title,
		Message:    "工单已提交，我们会尽快处理。",
		HasContact: req.Contact != "",
	})
}

func (h *Handler) ListPublicCategories(w http.ResponseWriter, r *http.Request) {
	h.json(w, 200, map[string]any{
		"items": h.snapshotCategories(),
	})
}
