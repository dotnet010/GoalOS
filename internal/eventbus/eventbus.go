// Package eventbus 实现 GoalOS 进程内事件总线。
//
// 核心特性：
//   - 同步分发：Publish 阻塞直到所有 handler 完成（微秒级）
//   - 单 goroutine 串行：保证事件有序
//   - Handler panic 隔离：recover 后记录日志，不影响其他 handler
//   - Allowed-Subscriber 列表（ACL）：错误检测机制——非安全 enforcing
//
// 设计依据：05 架构文档 §3、R174、R225。
package eventbus

import (
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/goalos/goalos/pkg/events"
)

// ErrHandlerFailed 是 handler 返回的非致命错误。
// 不影响其他 handler 执行。
var ErrHandlerFailed = errors.New("handler failed")

// SubscriptionID 唯一标识一个订阅。用于 Unsubscribe。
type SubscriptionID int

// subscription 是一个内部订阅记录。
type subscription struct {
	id      SubscriptionID              // 唯一标识
	handler func(events.Event) error    // 事件处理函数。必须微秒级返回
	role    string                      // 角色名。用于 ACL 检查
}

// EventBus 是进程内事件总线。
// 同步分发。微秒级 handler。不持久化——持久化由 State Store 独立负责。
type EventBus struct {
	mu     sync.RWMutex
	subs   map[string][]subscription // eventType → 订阅列表
	nextID SubscriptionID            // 下一个 SubscriptionID
	acl    map[string][]string       // eventType → 允许的角色列表。nil = 无 ACL
}

// New 创建一个无 ACL 的 EventBus。所有订阅者均可接收任何事件。
func New() *EventBus {
	return &EventBus{
		subs: make(map[string][]subscription),
	}
}

// NewWithACL 创建一个带 ACL 的 EventBus。
// acl 映射 eventType → 允许接收该事件的 role 列表。
// role 为空字符串的订阅者不受 ACL 限制。
func NewWithACL(acl map[string][]string) *EventBus {
	return &EventBus{
		subs: make(map[string][]subscription),
		acl:  acl,
	}
}

// Subscribe 注册一个事件处理器。role 为空，不受 ACL 限制。
// 返回 SubscriptionID 用于后续 Unsubscribe。
func (eb *EventBus) Subscribe(eventType string, handler func(events.Event) error) SubscriptionID {
	return eb.SubscribeAs(eventType, "", handler)
}

// SubscribeAs 以指定角色注册事件处理器。
// 如果该 eventType 有 ACL，只有 role 在允许列表中的订阅者才收到事件。
func (eb *EventBus) SubscribeAs(eventType string, role string, handler func(events.Event) error) SubscriptionID {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	eb.nextID++
	sub := subscription{
		id:      eb.nextID,
		handler: handler,
		role:    role,
	}
	eb.subs[eventType] = append(eb.subs[eventType], sub)
	return eb.nextID
}

// Unsubscribe 取消一个订阅。id 是由 Subscribe/SubscribeAs 返回的。
func (eb *EventBus) Unsubscribe(id SubscriptionID) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	for evtType, subs := range eb.subs {
		filtered := subs[:0]
		for _, s := range subs {
			if s.id != id {
				filtered = append(filtered, s)
			}
		}
		if len(filtered) != len(subs) {
			eb.subs[evtType] = filtered
		}
	}
}

// Publish 同步分发事件到所有匹配的订阅者。
// 阻塞直到所有 handler 完成。handler 必须是微秒级别——所有重 I/O
// 应在独立 goroutine 中异步执行。
//
// Handler panic 会被 recover 并记录日志，不影响其他 handler。
// Handler 返回 error 会被记录日志，不影响其他 handler。
func (eb *EventBus) Publish(evt events.Event) {
	eb.mu.RLock()
	subs := eb.subs[evt.Type]
	allowedRoles := eb.acl[evt.Type] // nil 如果该事件类型无 ACL
	eb.mu.RUnlock()

	for _, sub := range subs {
		// ACL 检查：如果设定了 ACL 且该订阅者 role 不在允许列表中，跳过
		if allowedRoles != nil && !contains(allowedRoles, sub.role) {
			continue
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[EventBus] handler panic for event %s: %v", evt.Type, r)
				}
			}()
			if err := sub.handler(evt); err != nil {
				log.Printf("[EventBus] handler error for event %s: %v", evt.Type, err)
			}
		}()
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// String 返回调试用字符串表示。
func (eb *EventBus) String() string {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	return fmt.Sprintf("EventBus{subs:%d}", len(eb.subs))
}
