// Package missionengine — Cloud LLM 客户端。
// 通过 OpenAI 兼容 API 调用云端 LLM（OpenAI、Anthropic 等）。
// 使用 Function Calling（Tools）+ jsonschema 获取结构化 MissionGraph。
// 内置错误分类、指数退避重试和 Token 使用追踪。
//
// 设计依据：05架构 §A.8、R243-R246、go-openai best practices。
package missionengine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"strconv"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"

	"github.com/goalos/goalos/internal/llm"
)

// CloudLLMClient 通过 OpenAI 兼容 API 调用云端 LLM。
// 支持 OpenAI、Anthropic（通过 DefaultAnthropicConfig）以及任何兼容 Provider。
type CloudLLMClient struct {
	model     string
	client    *openai.Client
	baseURL   string
	apiKey    string
	maxTokens int
}

// NewCloudLLMClient 创建云端 LLM 客户端。
// 自动检测 Provider 类型并配置相应的 API 端点。
// 自动获取模型上下文长度。
func NewCloudLLMClient(baseURL, apiKey, model string) *CloudLLMClient {
	var cfg openai.ClientConfig
	if baseURL != "" {
		cfg = openai.DefaultConfig(apiKey)
		cfg.BaseURL = baseURL
	} else {
		// 默认使用 Anthropic 兼容端点
		cfg = openai.DefaultAnthropicConfig(apiKey, "https://api.anthropic.com/v1")
	}
	c := &CloudLLMClient{
		model:   model,
		client:  openai.NewClientWithConfig(cfg),
		baseURL: baseURL,
		apiKey:  apiKey,
	}
	c.maxTokens = c.fetchContextLength()
	return c
}

// fetchContextLength 从 Provider API 获取模型的上下文窗口大小。
// 减去 4096 作为安全余量（留给 system prompt + response overhead）。
func (c *CloudLLMClient) fetchContextLength() int {
	targetURL := c.baseURL
	if targetURL == "" {
		targetURL = "https://api.anthropic.com/v1"
	}
	req, err := http.NewRequest("GET", targetURL+"/models/"+c.model, nil)
	if err != nil {
		return 32768
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("x-api-key", c.apiKey) // Anthropic 兼容
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 32768
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Data struct{ ContextLength json.RawMessage } `json:"data"`
		// Anthropic 格式
		ContextWindow *int `json:"context_window"`
	}
	// 尝试 OpenAI 格式
	if json.Unmarshal(body, &result) == nil {
		if n, err := strconv.Atoi(string(result.Data.ContextLength)); err == nil && n > 0 {
			return n - 4096
		}
	}
	// 尝试 Anthropic 格式
	if json.Unmarshal(body, &result) == nil && result.ContextWindow != nil && *result.ContextWindow > 0 {
		return *result.ContextWindow - 4096
	}
	return 32768
}

// planFunctionSchema 是 plan_goal 函数的 JSON Schema 定义。
// 使用 jsonschema 包从 PlanGoalParams 结构体自动生成。
func planFunctionSchema() *jsonschema.Definition {
	return llm.GeneratePlanSchema()
}

// planTool 返回 Function Calling 的 Tool 定义。
func planTool() openai.Tool {
	return openai.Tool{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        "plan_goal",
			Description: "将用户目标拆解为可执行任务图。每个节点指定 action_type 和 target。",
			Parameters:  planFunctionSchema(),
		},
	}
}

// Chat 调用云端 LLM，使用 Function Calling 获取结构化 MissionGraph。
// 实现 llm.Chat 接口。内置重试和错误分类。
func (c *CloudLLMClient) Chat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	cfg := llm.DefaultRetryConfig()
	return llm.WithRetry(ctx, cfg, func(attempt int) (*llm.ChatResponse, error) {
		return c.chatOnce(ctx, req)
	})
}

// chatOnce 执行单次 LLM API 调用（不含重试）。
func (c *CloudLLMClient) chatOnce(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	// 构建消息列表
	msgs := make([]openai.ChatCompletionMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = openai.ChatCompletionMessage{
			Role:    m.Role,
			Content: m.Content,
		}
	}

	maxTokens := c.maxTokens
	if req.MaxTokens > 0 && req.MaxTokens < maxTokens {
		maxTokens = req.MaxTokens
	}

	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: c.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: msgs[0].Content},
			{Role: openai.ChatMessageRoleUser, Content: msgs[1].Content},
		},
		MaxTokens:   maxTokens,
		Temperature: llm.SafeTemperature(0.3),
		Seed:        func() *int { s := 42; return &s }(),
		Tools:       []openai.Tool{planTool()},
		ToolChoice:  req.ToolChoice,
	})
	if err != nil {
		return nil, fmt.Errorf("cloud llm: %w", err)
	}

	// 提取 Function Calling 返回的 JSON
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("cloud llm: empty response")
	}
	msg := resp.Choices[0].Message

	response := &llm.ChatResponse{
		Content: msg.Content,
		Usage: llm.TokenUsage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}

	if len(msg.ToolCalls) > 0 {
		response.Content = msg.ToolCalls[0].Function.Arguments
		response.ToolCalls = make([]llm.ToolCall, len(msg.ToolCalls))
		for i, tc := range msg.ToolCalls {
			response.ToolCalls[i] = llm.ToolCall{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			}
		}
	} else if msg.Content != "" {
		// v1.1.0: 部分 LLM 不返回 ToolCalls，JSON 直接在 Content 中（可能含转义）
		if idx := strings.Index(msg.Content, `{"nodes"`); idx >= 0 {
			content := msg.Content[idx:]
			if end := strings.LastIndex(content, "}"); end > 0 {
				content = content[:end+1]
			}
			response.Content = content
		}
	}

	// Token 使用追踪写入日志
	if resp.Usage.TotalTokens > 0 {
		_ = resp.Usage // Token 追踪在上层（GoalAgent）处理
	}

	return response, nil
}

// ChatStream 返回流式 LLM 响应 channel。
func (c *CloudLLMClient) ChatStream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.ChatStreamEvent, error) {
	msgs := make([]openai.ChatCompletionMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = openai.ChatCompletionMessage{
			Role:    m.Role,
			Content: m.Content,
		}
	}

	maxTokens := c.maxTokens
	if req.MaxTokens > 0 && req.MaxTokens < maxTokens {
		maxTokens = req.MaxTokens
	}

	stream, err := c.client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model: c.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: msgs[0].Content},
			{Role: openai.ChatMessageRoleUser, Content: msgs[1].Content},
		},
		MaxTokens:   maxTokens,
		Temperature: llm.SafeTemperature(0.3),
		Seed:        func() *int { s := 42; return &s }(),
		Stream:      true,
		Tools:       []openai.Tool{planTool()},
	})
	if err != nil {
		return nil, fmt.Errorf("cloud llm stream: %w", err)
	}

	ch := make(chan llm.ChatStreamEvent, 10)
	go func() {
		defer stream.Close()
		defer close(ch)
		for {
			response, err := stream.Recv()
			if err != nil {
				// io.EOF 表示正常结束
				doneEvent := llm.ChatStreamEvent{Done: true}
				if err.Error() != "EOF" {
					doneEvent.Err = err
				}
				ch <- doneEvent
				return
			}
			if len(response.Choices) > 0 {
				delta := response.Choices[0].Delta
				event := llm.ChatStreamEvent{
					Content: delta.Content,
				}
				for _, tc := range delta.ToolCalls {
					event.ToolCalls = append(event.ToolCalls, llm.ToolCall{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					})
				}
				ch <- event
			}
		}
	}()

	return ch, nil
}

// SetTimeout 设置 API 调用超时（用于兼容旧接口，暂不实现）。
// 调用者应通过 ctx 参数控制超时。
func (c *CloudLLMClient) SetTimeout(d time.Duration) {
	// ctx-based timeout 替代了此功能
}
