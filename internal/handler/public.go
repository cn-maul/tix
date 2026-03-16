package handler

import (
	"net/http"
	"strings"
	"time"
	"tix/internal/model"

	captchaCommon "github.com/tianaiyouqing/tianai-captcha-go/common"
	captchaModel "github.com/tianaiyouqing/tianai-captcha-go/common/model"
)

func (h *Handler) GetPublicCaptcha(w http.ResponseWriter, r *http.Request) {
	captcha, err := h.captchaApp.GenerateCaptcha(&captchaModel.GenerateParam{
		CaptchaName: captchaCommon.CAPTCHA_NAME_SLIDER,
	})
	if err != nil {
		h.error(w, 500, "CAPTCHA_GENERATE_FAILED", "验证码生成失败")
		return
	}

	var sliderY any
	if data, ok := captcha.Data.(map[string]any); ok {
		sliderY = data["sliderY"]
	}
	h.json(w, 200, map[string]any{
		"id":                      captcha.Id,
		"type":                    captcha.CaptchaName,
		"background_image":        captcha.BackgroundImage,
		"template_image":          captcha.TemplateImage,
		"background_image_width":  captcha.BackgroundImageWidth,
		"background_image_height": captcha.BackgroundImageHeight,
		"template_image_width":    captcha.TemplateImageWidth,
		"template_image_height":   captcha.TemplateImageHeight,
		"slider_y":                sliderY,
	})
}

func (h *Handler) VerifyPublicCaptcha(w http.ResponseWriter, r *http.Request) {
	var req model.PublicCaptchaVerifyReq
	if err := h.decodeJSON(r, &req); err != nil {
		h.error(w, 400, "INVALID_JSON", "Invalid JSON body")
		return
	}

	req.ID = strings.TrimSpace(req.ID)
	if req.ID == "" {
		h.error(w, 400, "INVALID_CAPTCHA_ID", "验证码 ID 不能为空")
		return
	}
	if req.X < 0 {
		h.error(w, 400, "INVALID_CAPTCHA_TRACK", "滑块位置无效")
		return
	}

	now := time.Now().UnixMilli()
	x := float32(req.X)
	y := float32(0)
	trackType := "move"
	bgWidth := req.BgWidth
	if bgWidth <= 0 {
		bgWidth = 600
	}
	track := &captchaModel.ImageCaptchaTrack{
		BgImageWidth: &bgWidth,
		StartTime:    &now,
		StopTime:     &now,
		TrackList: []captchaModel.Track{
			{
				X:    &x,
				Y:    &y,
				Type: &trackType,
			},
		},
	}

	result, err := h.captchaApp.Valid(req.ID, track)
	if err != nil {
		h.error(w, 500, "CAPTCHA_VERIFY_FAILED", "验证码校验失败")
		return
	}
	if result.Code != 200 {
		h.error(w, 400, "INVALID_CAPTCHA", result.Msg)
		return
	}

	h.markPublicCaptchaVerified(req.ID)
	h.json(w, 200, map[string]any{
		"verified": true,
		"message":  "验证通过",
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
	req.CaptchaID = strings.TrimSpace(req.CaptchaID)

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
	if req.CaptchaID == "" {
		h.error(w, 400, "INVALID_CAPTCHA", "请先完成滑块验证")
		return
	}
	if !h.consumeVerifiedPublicCaptcha(req.CaptchaID) {
		h.error(w, 400, "INVALID_CAPTCHA", "滑块验证无效或已过期，请重新验证")
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
