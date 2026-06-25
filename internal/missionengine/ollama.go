// Package missionengine — Ollama LLM 客户端。
// 通过 Ollama 的 OpenAI 兼容 API（/v1）调用本地 LLM。
// 使用 JSON mode 输出结构化 MissionGraph。
// 内置错误分类、指数退避重试和 Token 使用追踪。
//
// 设计依据：05架构 §A.8、R241、R243-R246。
package missionengine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
// 自动从 Ollama 获取模型上下文长度。
func NewOllamaClient(model, baseURL string) *OllamaClient {
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
	o.maxTokens = o.fetchContextLength()
	return o
}

// fetchContextLength 从 Ollama /api/show 端点获取模型的上下文窗口大小。
// 减去 4096 作为安全余量。
func (o *OllamaClient) fetchContextLength() int {
	resp, err := http.Post(o.baseURL+"/api/show", "application/json",
		strings.NewReader(`{"model":"`+o.model+`"}`))
	if err != nil {
		return 8192
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result struct {
		ModelInfo map[string]interface{} `json:"model_info"`
	}
	if json.Unmarshal(body, &result) != nil {
		return 8192
	}
	if ctxLen, ok := result.ModelInfo["context_length"].(float64); ok && ctxLen > 0 {
		return int(ctxLen) - 4096
	}
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
