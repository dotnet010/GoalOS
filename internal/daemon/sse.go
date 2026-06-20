// Package daemon — Server-Sent Events 实时推送。
// 纯 HTTP。零外部依赖。替代 WebSocket。
// 客户端连接 → 订阅 Event Bus → 事件发生时推送到浏览器。
//
// 设计依据：05 架构文档 §2.2、R33。
package daemon

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
)

// SSEManager 管理所有 SSE 连接。
type SSEManager struct {
	mu    sync.RWMutex
	conns map[chan string]bool
}

// NewSSEManager 创建 SSE 管理器。
func NewSSEManager() *SSEManager {
	return &SSEManager{
		conns: make(map[chan string]bool),
	}
}

// HandleSSE 处理 SSE 连接。GET /api/events。
func (sm *SSEManager) HandleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "不支持 SSE", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan string, 50)
	sm.mu.Lock()
	sm.conns[ch] = true
	sm.mu.Unlock()

	log.Printf("[SSE] 客户端已连接 (总连接: %d)", len(sm.conns))

	defer func() {
		sm.mu.Lock()
		delete(sm.conns, ch)
		sm.mu.Unlock()
		close(ch)
		log.Printf("[SSE] 客户端已断开")
	}()

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// Push 推送事件到所有 SSE 客户端。
func (sm *SSEManager) Push(eventType string, payload map[string]interface{}) {
	data, _ := json.Marshal(map[string]interface{}{
		"type":    eventType,
		"payload": payload,
	})

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for ch := range sm.conns {
		select {
		case ch <- string(data):
		default:
			// 客户端太慢——跳过
		}
	}
}

// Count 返回当前连接数。
func (sm *SSEManager) Count() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.conns)
}
