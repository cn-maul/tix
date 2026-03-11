package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"
)

type AIClient struct {
	APIKey  string
	BaseURL string
	Model   string
}

type AIResult struct {
	Title    string
	Category string
}

func NewAIClient(apiKey, baseURL, model string) *AIClient {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if model == "" {
		model = "gpt-3.5-turbo"
	}
	return &AIClient{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   model,
	}
}

func (c *AIClient) IsEnabled() bool {
	return c.APIKey != ""
}

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// AnalyzeContent 用AI分析工单内容，生成标题和分类
func (c *AIClient) AnalyzeContent(content string, categories []string) AIResult {
	// 默认分类（如果categories为空则返回空字符串）
	defaultCategory := ""
	if len(categories) > 0 {
		defaultCategory = categories[0]
	}

	if !c.IsEnabled() {
		return AIResult{
			Title:    defaultTitle(content),
			Category: defaultCategory,
		}
	}

	categoryList := strings.Join(categories, "、")
	prompt := fmt.Sprintf(`请分析以下工单内容，完成两个任务：

1. 生成一个简短的标题（不超过15个字）
2. 从以下分类中选择最合适的一个：%s

请按以下JSON格式输出，不要其他内容：
{"title":"标题内容","category":"选择的分类"}

工单内容：
%s`, categoryList, content)

	req := chatRequest{
		Model: c.Model,
		Messages: []message{
			{Role: "user", Content: prompt},
		},
	}

	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequest("POST", c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return AIResult{Title: defaultTitle(content), Category: defaultCategory}
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return AIResult{Title: defaultTitle(content), Category: defaultCategory}
	}
	defer resp.Body.Close()

	var chatResp chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return AIResult{Title: defaultTitle(content), Category: defaultCategory}
	}

	if len(chatResp.Choices) == 0 {
		return AIResult{Title: defaultTitle(content), Category: defaultCategory}
	}

	// 解析JSON响应
	result := chatResp.Choices[0].Message.Content
	result = strings.TrimSpace(result)
	result = strings.Trim(result, "`")

	var aiResult AIResult
	if err := json.Unmarshal([]byte(result), &aiResult); err != nil {
		// 如果解析失败，尝试从文本中提取
		return AIResult{Title: defaultTitle(content), Category: defaultCategory}
	}

	// 验证分类是否有效
	validCategory := slices.Contains(categories, aiResult.Category)
	if !validCategory && len(categories) > 0 {
		aiResult.Category = categories[0]
	}

	// 截断标题
	if len([]rune(aiResult.Title)) > 20 {
		aiResult.Title = string([]rune(aiResult.Title)[:20])
	}

	return aiResult
}

// GenerateTitle 仅生成标题（兼容旧代码）
func (c *AIClient) GenerateTitle(content string) string {
	return c.AnalyzeContent(content, []string{"硬件故障", "网络问题", "软件支持", "会议设备"}).Title
}

func defaultTitle(content string) string {
	runes := []rune(content)
	if len(runes) <= 5 {
		return content
	}
	return string(runes[:5])
}
