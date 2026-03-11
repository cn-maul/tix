package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"tix/internal/config"
)

type AIService struct {
	cfg *config.AIConfig
}

func NewAIService(cfg *config.AIConfig) *AIService {
	if cfg == nil || cfg.APIKey == "" {
		return nil
	}
	return &AIService{cfg: cfg}
}

type aiChatRequest struct {
	Model    string       `json:"model"`
	Messages []aiMessage  `json:"messages"`
	Stream   bool         `json:"stream"`
}

type aiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type aiChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (s *AIService) callAPI(prompt string) (string, error) {
	if s == nil || s.cfg.APIKey == "" {
		return "", errors.New("AI not configured")
	}

	baseURL := s.cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	model := s.cfg.Model
	if model == "" {
		model = "gpt-3.5-turbo"
	}

	req := aiChatRequest{
		Model: model,
		Messages: []aiMessage{
			{Role: "user", Content: prompt},
		},
		Stream: false,
	}

	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequest("POST", baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var chatResp aiChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", err
	}

	if chatResp.Error != nil {
		return "", errors.New(chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return "", errors.New("no response from AI")
	}

	return strings.TrimSpace(chatResp.Choices[0].Message.Content), nil
}

func (s *AIService) GenerateTitle(content string) (string, error) {
	// 截取前200字
	runes := []rune(content)
	if len(runes) > 200 {
		content = string(runes[:200])
	}

	prompt := fmt.Sprintf(`请为以下工单生成一个简短的标题（不超过15个字），只输出标题，不要其他内容：

工单内容：
%s`, content)

	result, err := s.callAPI(prompt)
	if err != nil {
		// fallback
		if len(runes) > 15 {
			return string(runes[:15]), nil
		}
		return content, nil
	}

	// 截断
	if len([]rune(result)) > 20 {
		result = string([]rune(result)[:20])
	}

	return result, nil
}

func (s *AIService) SelectCategory(content string) (string, error) {
	if s == nil {
		return "", errors.New("AI not configured")
	}

	// 这里需要从配置获取分类，暂时返回空
	// 实际使用时应该传入categories
	return "", nil
}

func (s *AIService) SelectCategoryFromList(content string, categories []string) (string, error) {
	if len(categories) == 0 {
		return "", nil
	}

	categoryList := strings.Join(categories, "、")
	prompt := fmt.Sprintf(`请从以下分类中选择最适合的一个（只输出分类名称，不要其他内容）：

分类列表：%s

工单内容：
%s`, categoryList, content)

	result, err := s.callAPI(prompt)
	if err != nil {
		return categories[0], nil
	}

	// 验证
	if slices.Contains(categories, result) {
		return result, nil
	}

	return categories[0], nil
}

func (s *AIService) Test(apiKey, baseURL, model string) (string, error) {
	// 创建临时client测试
	testService := &AIService{
		cfg: &config.AIConfig{
			APIKey:  apiKey,
			BaseURL: baseURL,
			Model:   model,
		},
	}

	return testService.callAPI("请回复'OK'")
}
