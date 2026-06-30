// Package missionengine — Ollama LLM 客户端。
// 通过 Ollama 的 OpenAI 兼容 API（/v1）调用本地 LLM。
// 使用 JSON mode 输出结构化 MissionGraph。
// 内置错误分类、指数退避重试和 Token 使用追踪。
//
// 设计依据：05架构 §A.8、R241、R243-R246。
package missionengine

import (
	"context"
	"fmt"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"

	"github.com/goalos/goalos/internal/llm"
)

// OllamaClient 通过 Ollama 的 OpenAI 兼容 API 调用本地 LLM。
// baseURL 可通过配置文件或环境变量 OLLAMA_BASE_URL 设置。
type OllamaClient struct {
	model     string
	client    *openai.Client
	baseURL   string // 完整的 Ollama 基础 URL
	maxTokens int
}

// NewOllamaClient 创建 Ollama 客户端。
// baseURL: Ollama 服务基础 URL（如 "http://localhost:11434"）。
// maxTokens: 显式配置的最大生成 Token 数。0 表示自动推断。
func NewOllamaClient(model, baseURL string, maxTokens int) *OllamaClient {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	cfg := openai.DefaultConfig("ollama")
	cfg.BaseURL = baseURL + "/v1"
	o := &OllamaClient{
		model:   model,
		client:  openai.NewClientWithConfig(cfg),
		baseURL: baseURL,
	}
	if maxTokens > 0 {
		o.maxTokens = maxTokens
	} else {
		o.maxTokens = inferOllamaMaxTokens(model)
	}
	return o
}

// inferOllamaMaxTokens 根据模型名推断合理的 maxTokens。
// 基于公开的模型上下文窗口规格，预留 4096 prompt 余量。
// 未知模型返回保守默认值 8192。
func inferOllamaMaxTokens(model string) int {
	// 按模型系列匹配（大小写不敏感前缀匹配）
	m := strings.ToLower(model)

	// 128K 上下文窗口系列
	if strings.Contains(m, "llama3.2") || strings.Contains(m, "llama3.3") ||
		strings.Contains(m, "llama3-") || strings.Contains(m, "llama-4") ||
		strings.Contains(m, "command-r") {
		return 124000 // 128K - 4K prompt 余量
	}
	if strings.Contains(m, "llama3") || strings.Contains(m, "llama-3") {
		return 124000 // Llama 3 系列: 128K 上下文
	}

	// 32K 上下文窗口系列
	if strings.Contains(m, "mistral") || strings.Contains(m, "mixtral") {
		return 28000 // 32K - 4K 余量
	}
	if strings.Contains(m, "codestral") || strings.Contains(m, "nemo") {
		return 28000
	}

	// Qwen 系列
	if strings.Contains(m, "qwen2.5") || strings.Contains(m, "qwen3") {
		return 124000 // Qwen 2.5/3: 128K 上下文
	}
	if strings.Contains(m, "qwen2") || strings.Contains(m, "qwen") {
		return 28000
	}

	// DeepSeek 系列
	if strings.Contains(m, "deepseek-v3") || strings.Contains(m, "deepseek-r1") {
		return 60000 // DeepSeek V3/R1: ~64K 上下文
	}
	if strings.Contains(m, "deepseek") {
		return 28000
	}

	// Gemma 系列
	if strings.Contains(m, "gemma3") || strings.Contains(m, "gemma-3") {
		return 124000
	}
	if strings.Contains(m, "gemma2") || strings.Contains(m, "gemma") {
		return 7800 // Gemma 2: 8K 上下文 - 200 余量
	}

	// Phi 系列
	if strings.Contains(m, "phi-4") || strings.Contains(m, "phi4") {
		return 124000
	}
	if strings.Contains(m, "phi-3") || strings.Contains(m, "phi3") {
		return 124000
	}
	if strings.Contains(m, "phi") {
		return 3700 // Phi-2: 4K 上下文 - 300 余量; 保守
	}

	// Yi 系列
	if strings.Contains(m, "yi-") {
		return 195000 // Yi-Large: 200K
	}

	// 保守默认值——未知模型
	return 8192
}

// Chat 调用 Ollama LLM，返回结构化响应。
// 实现 LLMClient 接口。内置重试和错误分类。
func (o *OllamaClient) Chat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	cfg := llm.DefaultRetryConfig()
	return llm.WithRetry(ctx, cfg, func(attempt int) (*llm.ChatResponse, error) {
		return o.chatOnce(ctx, req)
	})
}

// chatOnce 执行单次 Ollama API 调用（不含重试）。
func (o *OllamaClient) chatOnce(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	msgs := make([]openai.ChatCompletionMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = openai.ChatCompletionMessage{
			Role:    m.Role,
			Content: m.Content,
		}
	}

	maxTokens := o.maxTokens
	if req.MaxTokens > 0 && req.MaxTokens < maxTokens {
		maxTokens = req.MaxTokens
	}

	resp, err := o.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: o.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: msgs[0].Content},
			{Role: openai.ChatMessageRoleUser, Content: msgs[1].Content},
		},
		MaxTokens: maxTokens,
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("ollama: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("ollama: empty response")
	}

	return &llm.ChatResponse{
		Content: resp.Choices[0].Message.Content,
		Usage: llm.TokenUsage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}, nil
}

// ChatStream 返回流式 Ollama 响应 channel。
func (o *OllamaClient) ChatStream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.ChatStreamEvent, error) {
	msgs := make([]openai.ChatCompletionMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = openai.ChatCompletionMessage{
			Role:    m.Role,
			Content: m.Content,
		}
	}

	maxTokens := o.maxTokens
	if req.MaxTokens > 0 && req.MaxTokens < maxTokens {
		maxTokens = req.MaxTokens
	}

	stream, err := o.client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model: o.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: msgs[0].Content},
			{Role: openai.ChatMessageRoleUser, Content: msgs[1].Content},
		},
		MaxTokens: maxTokens,
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		},
		Stream: true,
	})
	if err != nil {
		return nil, fmt.Errorf("ollama stream: %w", err)
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
				ch <- llm.ChatStreamEvent{
					Content: response.Choices[0].Delta.Content,
				}
			}
		}
	}()

	return ch, nil
}

// SetTimeout 设置 API 调用超时（用于兼容旧接口，暂不实现）。
// 调用者应通过 ctx 参数控制超时。
func (o *OllamaClient) SetTimeout(d time.Duration) {
	// ctx-based timeout 替代了此功能
}
