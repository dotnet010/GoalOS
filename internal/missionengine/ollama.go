// Package missionengine — Ollama LLM 客户端。
// 实现 LLMClient 接口。调用本地 Ollama API (localhost:11434)。
package missionengine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OllamaClient 通过 Ollama API 调用本地 LLM。
type OllamaClient struct {
	Model  string // 模型名，如 "qwen3.5:0.8b"
	apiURL string
	client *http.Client
}

// NewOllamaClient 创建 Ollama 客户端。
func NewOllamaClient(model string) *OllamaClient {
	return &OllamaClient{
		Model:  model,
		apiURL: "http://localhost:11434/api/generate",
		client: &http.Client{Timeout: 300 * time.Second},
	}
}

// Chat 调用 Ollama API，返回模型响应。
func (o *OllamaClient) Chat(systemPrompt string, userMessage string) (string, error) {
	// Ollama 用 system prompt 作为完整 prompt 的前缀
	fullPrompt := systemPrompt + "\n\n" + userMessage

	reqBody := map[string]interface{}{
		"model":  o.Model,
		"prompt": fullPrompt,
		"stream": false,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("ollama: marshal: %w", err)
	}

	resp, err := o.client.Post(o.apiURL, "application/json", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("ollama: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama: %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("ollama: decode: %w", err)
	}

	return result.Response, nil
}
