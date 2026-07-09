// Package scheduler — v0.2.0 Week 8 B15: Two-Phase Commit 原子事件存储
// 保证: events.jsonl append → fsync → EventBus Publish。
// 投影不可先于真相落盘。
package scheduler

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// AtomicEventStore 提供原子事件追加——先 fsync 再 Publish。
type AtomicEventStore struct {
	mu   sync.Mutex
	path string
}

// NewAtomicEventStore 创建原子事件存储。
func NewAtomicEventStore(path string) *AtomicEventStore {
	return &AtomicEventStore{path: path}
}

// eventEntry 是 events.jsonl 中的一行。
type eventEntry struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

// Append 追加事件到 events.jsonl 并 fsync。
// 返回 error 如果写入失败。
func (s *AtomicEventStore) Append(eventType, data string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := eventEntry{Type: eventType, Data: data}
	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("json marshal: %w", err)
	}
	line = append(line, '\n')

	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open events.jsonl: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(line); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	// B15: fsync 强制落盘——在 Publish 之前
	if err := f.Sync(); err != nil {
		return fmt.Errorf("fsync: %w", err)
	}
	return nil
}

// AppendAndPublish 追加事件→fsync→成功后执行 publish 回调。
// B15 TPC: publish 仅在 Append+fsync 成功后执行。
func (s *AtomicEventStore) AppendAndPublish(eventType, data string, publish func()) error {
	if err := s.Append(eventType, data); err != nil {
		return err
	}
	if publish != nil {
		publish()
	}
	return nil
}
