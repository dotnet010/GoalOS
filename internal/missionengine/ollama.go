// Package missionengine — LLM 客户端。
// 通过 OpenAI 兼容 API 调用本地 Ollama（或任何兼容 Provider）。
// 设计依据：05架构文档 §A.8、ADR-013。any-llm-go 不可用→使用 go-openai。
package missionengine

import (
	"context"
	"fmt"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// OllamaClient 通过 Ollama 的 OpenAI 兼容 API 调用本地 LLM。
type OllamaClient struct {
	model  string
	client *openai.Client
}

// NewOllamaClient 创建客户端。Ollama 在 localhost:11434/v1 提供 OpenAI 兼容端点。
func NewOllamaClient(model string) *OllamaClient {
	cfg := openai.DefaultConfig("ollama") // key 可为任意值，Ollama 忽略
	cfg.BaseURL = "http://localhost:11434/v1"
	return &OllamaClient{
		model:  model,
		client: openai.NewClientWithConfig(cfg),
	}
}

// Chat 调用 LLM，返回响应文本。实现 LLMClient 接口。
func (o *OllamaClient) Chat(systemPrompt string, userMessage string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()

	resp, err := o.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: o.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: userMessage},
		},
		MaxTokens: 8192,
	})
	if err != nil {
		return "", fmt.Errorf("ollama: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("ollama: empty response")
	}
	return resp.Choices[0].Message.Content, nil
}
