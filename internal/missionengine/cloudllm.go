// Package missionengine — Cloud LLM 客户端。
// 通过 OpenAI 兼容 API 调用云端 LLM（OpenAI、Anthropic、百炼、OpenRouter 等）。
// 使用 Function Calling（Tools）+ jsonschema 获取结构化 MissionGraph。
// 内置错误分类、指数退避重试和 Token 使用追踪。
//
// 设计依据：05架构 §A.8、R243-R246、go-openai best practices。
package missionengine

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"

	"github.com/goalos/goalos/internal/llm"
)

// CloudLLMClient 通过 OpenAI 兼容 API 调用云端 LLM。
// 支持 OpenAI、Anthropic、百炼、OpenRouter 以及任何兼容 Provider。
// 通过 go-openai SDK 统一封装，只需配置 BaseURL 即可切换 Provider。
type CloudLLMClient struct {
	model     string
	client    *openai.Client
	baseURL   string
	apiKey    string
	maxTokens int
}

// NewCloudLLMClient 创建云端 LLM 客户端。
// 使用 go-openai 原生配置支持任意 OpenAI 兼容 Provider：
//   - OpenAI:    baseURL="" (使用 DefaultConfig)
//   - Anthropic: baseURL="" (使用 DefaultAnthropicConfig)
//   - 百炼:       baseURL="https://dashscope.aliyuncs.com/compatible-mode/v1"
//   - OpenRouter: baseURL="https://openrouter.ai/api/v1"
//   - 其他:       baseURL="自定义兼容端点"
//
// maxTokens 由调用方配置（常用值：4096, 8192, 16384, 32768, 128000）。
// [FIXED] 不再动态获取模型上下文长度——go-openai 不提供此 API，
// 且动态获取增加了不必要的复杂性和失败点。
func NewCloudLLMClient(baseURL, apiKey, model string, maxTokens int) *CloudLLMClient {
	var cfg openai.ClientConfig
	if baseURL == "" {
		// 默认使用 Anthropic 兼容端点
		cfg = openai.DefaultAnthropicConfig(apiKey, "https://api.anthropic.com/v1")
	} else {
		// 任意 OpenAI 兼容 Provider（百炼、OpenRouter、本地模型等）
		cfg = openai.DefaultConfig(apiKey)
		cfg.BaseURL = baseURL
	}

	return &CloudLLMClient{
		model:     model,
		client:    openai.NewClientWithConfig(cfg),
		baseURL:   baseURL,
		apiKey:    apiKey,
		maxTokens: maxTokens,
	}
}

// [DELETED] fetchContextLength — 已删除
// 原因：
// 1. go-openai SDK 不提供获取模型信息的 API
// 2. 自己构造 HTTP 请求脆弱（硬编码端点、忽略错误、格式不兼容）
// 3. 动态获取增加了不必要的失败点
// 4. maxTokens 应由调用方根据业务需求和模型能力配置

// planFunctionSchema 是 plan_goal 函数的 JSON Schema 定义。
// 使用 go-openai 的 jsonschema 包从 PlanGoalParams 结构体自动生成。
func planFunctionSchema() *jsonschema.Definition {
	return llm.GeneratePlanSchema()
}

// planTool 返回 Function Calling 的 Tool 定义。
// 使用 go-openai 原生类型 openai.Tool。
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
// 底层使用 go-openai 的 CreateChatCompletion 发送 HTTP 请求。
func (c *CloudLLMClient) Chat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	goalID := ""
	if ctx.Value("goalID") != nil {
		goalID = ctx.Value("goalID").(string)
	}
	log.Printf("[TRACE] %s ├─ LLM.Chat → %s (%s) msgs=%d", goalID, c.model, c.baseURL, len(req.Messages))

	cfg := llm.DefaultRetryConfig()
	result, err := llm.WithRetry(ctx, cfg, func(attempt int) (*llm.ChatResponse, error) {
		log.Printf("[TRACE] %s │  LLM attempt %d", goalID, attempt)
		return c.chatOnce(ctx, req)
	})
	if err != nil {
		log.Printf("[TRACE] %s ├─ LLM.Chat ❌ %v", goalID, err)
	} else {
		log.Printf("[TRACE] %s ├─ LLM.Chat ✅ (%d chars)", goalID, len(result.Content))
	}
	return result, err
}

// chatOnce 执行单次 LLM API 调用（不含重试）。
// 使用 go-openai 原生方法 CreateChatCompletion。
func (c *CloudLLMClient) chatOnce(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	// 将 GoalOS 的 Message 转换为 go-openai 的 ChatCompletionMessage
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

	// R-724: Function Calling 兼容性——部分 Provider（如百炼 glm-5.1）不支持 tool_choice。
	// 先尝试带 Tools 的请求；失败→降级为纯文本 prompt（无 Function Calling）。
	// go-openai 原生支持 Tools 和 ToolChoice 参数。
	result, err := c.tryChatWithTools(ctx, c.model, msgs, maxTokens)
	if err != nil {
		log.Printf("[CloudLLM] Function Calling failed (%v), falling back to plain text mode", err)
		result, err = c.tryChatPlain(ctx, c.model, msgs, maxTokens)
		if err != nil {
			return nil, fmt.Errorf("cloud llm: %w", err)
		}
	}
	return result, nil
}

// tryChatWithTools 尝试带 Function Calling 的 Chat 请求。
// 使用 go-openai 的 CreateChatCompletion 方法，传入 Tools 和 ToolChoice 参数。
func (c *CloudLLMClient) tryChatWithTools(ctx context.Context, model string, msgs []openai.ChatCompletionMessage, maxTokens int) (*llm.ChatResponse, error) {
	start := time.Now()
	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:       model,
		Messages:    msgs,
		MaxTokens:   maxTokens,
		Temperature: llm.SafeTemperature(0.3),
		Seed:        func() *int { s := 42; return &s }(),
		Tools:       []openai.Tool{planTool()},
		ToolChoice:  "required",
	})
	elapsed := time.Since(start)
	if err != nil {
		log.Printf("[CloudLLM] HTTP toolsCall FAILED after %v: %v", elapsed.Round(time.Millisecond), err)
		return nil, err
	}

	// [FIXED] 增强日志：包含 response ID 用于外部监控交叉验证
	log.Printf("[CloudLLM] HTTP toolsCall OK after %v: model=%s id=%s tokens=%d prompt=%d completion=%d",
		elapsed.Round(time.Millisecond),
		resp.Model,
		resp.ID,
		resp.Usage.TotalTokens,
		resp.Usage.PromptTokens,
		resp.Usage.CompletionTokens,
	)

	// [FIXED] 检测可疑响应（可能是 SDK Mock 或缓存命中）
	if resp.ID == "" {
		log.Printf("[CloudLLM] WARNING: response ID is empty — possible SDK mock response or cache hit")
	}
	if resp.Usage.TotalTokens == 0 {
		log.Printf("[CloudLLM] WARNING: total tokens is 0 — possible SDK mock response")
	}

	return c.parseResponse(resp), nil
}

// tryChatPlain 纯文本模式（无 Function Calling）。R-724: Provider 兼容性降级。
// 使用 go-openai 的 CreateChatCompletion 方法，不带 Tools 参数。
func (c *CloudLLMClient) tryChatPlain(ctx context.Context, model string, msgs []openai.ChatCompletionMessage, maxTokens int) (*llm.ChatResponse, error) {
	start := time.Now()
	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:       model,
		Messages:    msgs,
		MaxTokens:   maxTokens,
		Temperature: llm.SafeTemperature(0.3),
	})
	elapsed := time.Since(start)
	if err != nil {
		log.Printf("[CloudLLM] HTTP plainCall FAILED after %v: %v", elapsed.Round(time.Millisecond), err)
		return nil, err
	}

	log.Printf("[CloudLLM] HTTP plainCall OK after %v: model=%s id=%s tokens=%d prompt=%d completion=%d",
		elapsed.Round(time.Millisecond),
		resp.Model,
		resp.ID,
		resp.Usage.TotalTokens,
		resp.Usage.PromptTokens,
		resp.Usage.CompletionTokens,
	)

	if resp.ID == "" {
		log.Printf("[CloudLLM] WARNING: response ID is empty — possible SDK mock response or cache hit")
	}
	if resp.Usage.TotalTokens == 0 {
		log.Printf("[CloudLLM] WARNING: total tokens is 0 — possible SDK mock response")
	}

	content := resp.Choices[0].Message.Content
	return &llm.ChatResponse{Content: content, Usage: llm.TokenUsage{
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		TotalTokens:      resp.Usage.TotalTokens,
	}}, nil
}

// parseResponse 解析 go-openai 的 Function Calling 响应为 GoalOS 的 ChatResponse。
// go-openai 已经解析了 HTTP 响应，这里只需提取业务数据。
func (c *CloudLLMClient) parseResponse(resp openai.ChatCompletionResponse) *llm.ChatResponse {
	if len(resp.Choices) == 0 {
		return &llm.ChatResponse{Content: ""}
	}
	msg := resp.Choices[0].Message
	log.Printf("[CloudLLM] parseResponse: choices=%d toolCalls=%d contentLen=%d finishReason=%s",
		len(resp.Choices), len(msg.ToolCalls), len(msg.Content), resp.Choices[0].FinishReason)
	if len(msg.ToolCalls) > 0 {
		log.Printf("[CloudLLM] ToolCall[0]: name=%s argsLen=%d args=%.200s",
			msg.ToolCalls[0].Function.Name, len(msg.ToolCalls[0].Function.Arguments),
			msg.ToolCalls[0].Function.Arguments)
	}

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
		// v0.1.0: 部分 LLM 不返回 ToolCalls，JSON 直接在 Content 中（可能含转义）
		if idx := strings.Index(msg.Content, `{"nodes"`); idx >= 0 {
			content := msg.Content[idx:]
			if end := strings.LastIndex(content, "}"); end > 0 {
				content = content[:end+1]
			}
			response.Content = content
		}
	}

	return response
}

// ChatStream 返回流式 LLM 响应 channel。
// 使用 go-openai 的 CreateChatCompletionStream 方法。
// channel 管理是 GoalOS 业务逻辑，合理。
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
