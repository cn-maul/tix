package handler

import (
	"time"

	"github.com/tianaiyouqing/tianai-captcha-go/application"
	captchaCommon "github.com/tianaiyouqing/tianai-captcha-go/common"
	captchaModel "github.com/tianaiyouqing/tianai-captcha-go/common/model"
)

const publicCaptchaVerifyTTL = 5 * time.Minute

type publicCaptchaProcessor struct{}

func newPublicCaptchaApp() *application.TianAiCaptchaApplication {
	builder := application.NewBuilder()
	builder.AddProvider(application.CreateSliderProvider())
	builder.AddProcessor(publicCaptchaProcessor{})
	return builder.Build()
}

func (publicCaptchaProcessor) BeforeGenerateCaptchaImage(exchange *captchaModel.CaptchaExchange, _ *application.TianAiCaptchaApplication) (*captchaModel.ImageCaptchaInfo, error) {
	return nil, nil
}

func (publicCaptchaProcessor) BeforeWrapImageCaptchaInfo(exchange *captchaModel.CaptchaExchange, _ *application.TianAiCaptchaApplication) error {
	if exchange == nil || exchange.Param == nil || exchange.Param.CaptchaName != captchaCommon.CAPTCHA_NAME_SLIDER {
		return nil
	}
	pos, ok := exchange.TransferData.(map[string]int)
	if !ok {
		return nil
	}
	exchange.CustomData.ViewData["sliderY"] = pos["y"]
	return nil
}

func (publicCaptchaProcessor) AfterGenerateCaptchaImage(_ *captchaModel.CaptchaExchange, _ *captchaModel.ImageCaptchaInfo, _ *application.TianAiCaptchaApplication) error {
	return nil
}

func (h *Handler) markPublicCaptchaVerified(id string) {
	h.publicCaptchaMu.Lock()
	defer h.publicCaptchaMu.Unlock()

	h.pruneVerifiedCaptchasLocked(time.Now())
	h.publicVerifiedCaptcha[id] = time.Now().Add(publicCaptchaVerifyTTL)
}

func (h *Handler) consumeVerifiedPublicCaptcha(id string) bool {
	h.publicCaptchaMu.Lock()
	defer h.publicCaptchaMu.Unlock()

	now := time.Now()
	h.pruneVerifiedCaptchasLocked(now)
	expireAt, ok := h.publicVerifiedCaptcha[id]
	if !ok || now.After(expireAt) {
		delete(h.publicVerifiedCaptcha, id)
		return false
	}
	delete(h.publicVerifiedCaptcha, id)
	return true
}

func (h *Handler) pruneVerifiedCaptchasLocked(now time.Time) {
	for id, expireAt := range h.publicVerifiedCaptcha {
		if now.After(expireAt) {
			delete(h.publicVerifiedCaptcha, id)
		}
	}
}
