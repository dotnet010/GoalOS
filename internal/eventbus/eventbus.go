// Package eventbus 实现 GoalOS 进程内事件总线。两层架构（v0.1.0）。
//
// 核心层（同步）：
//   - 核心状态变更事件（GoalCreated, ActionCompleted, PipelinePaused 等）
//   - Publish 阻塞直到所有 handler 完成。handler < 1ms 返回
//   - 禁止在 handler 内进行 I/O、网络调用、审计写入
//
// 副作用层（异步）：
//   - 副作用事件（AuditLogWritten, TokenUsage, MetricsSnapshot, NotificationSent）
//   - goroutine pool 异步投递。副作用层失败不影响核心状态转换
//   - 缓冲 channel（100）。满时丢弃非关键事件
//
// 设计依据：05 架构文档 §3.6、R-321、R-342、R-349。
package eventbus

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/goalos/goalos/pkg/events"
)

// ErrHandlerFailed 是 handler 返回的非致命错误。
var ErrHandlerFailed = errors.New("handler failed")

// SubscriptionID 唯一标识一个订阅。用于 Unsubscribe。
type SubscriptionID int

// subscription 是一个内部订阅记录。
type subscription struct {
	id      SubscriptionID
	handler func(events.Event) error
	role    string // 角色名。用于 ACL 检查
	goalID  string // v0.1.0: per-goal 过滤。空="" 接收所有 Goal
	async   bool   // v0.1.0: true = 异步订阅（副作用层）
}

// EventBus 是进程内事件总线。两层架构（v0.1.0 R-349）。
type EventBus struct {
	mu     sync.RWMutex
	subs   map[string][]subscription // eventType → 订阅列表
	nextID SubscriptionID
	acl    map[string][]string // eventType → 允许的角色列表

	// ── 副作用层（v0.1.0 R-349）──
	asyncEvents map[string]bool    // 路由到异步路径的事件类型
	asyncCh     chan asyncPayload  // 缓冲 channel（100）
	asyncPool   int                // goroutine pool 大小
	ctx         context.Context    // 关闭信号
	cancel      context.CancelFunc
	wg          sync.WaitGroup     // 等待异步 workers 完成
}

// asyncPayload 是异步投递的载荷。
type asyncPayload struct {
	evt  events.Event
	subs []subscription
}

// asyncEventTypes 定义必须走副作用层的事件类型（R-349）。
var asyncEventTypes = map[string]bool{
	events.TypeAuditLogWritten:  true, // 审计写入——I/O 操作
	events.TypeTokenUsage:       true, // Token 统计——非状态变更
	events.TypeMetricsSnapshot:  true, // 指标采集——非状态变更
	events.TypeNotificationSent: true, // 通知推送——外部 I/O
}

const (
	defaultAsyncPool   = 5   // goroutine pool 大小（匹配最大并发 Goal 数）
	defaultAsyncBuffer = 100 // 缓冲 channel 容量
)

// New 创建一个无 ACL 的 EventBus。默认 5 worker 副作用层。
func New() *EventBus {
	ctx, cancel := context.WithCancel(context.Background())
	eb := &EventBus{
		subs:        make(map[string][]subscription),
		asyncEvents: asyncEventTypes,
		asyncPool:   defaultAsyncPool,
		ctx:         ctx,
		cancel:      cancel,
	}
	eb.asyncCh = make(chan asyncPayload, defaultAsyncBuffer)
	eb.startAsyncWorkers()
	return eb
}

// NewWithACL 创建一个带 ACL 的 EventBus。
func NewWithACL(acl map[string][]string) *EventBus {
	ctx, cancel := context.WithCancel(context.Background())
	eb := &EventBus{
		subs:        make(map[string][]subscription),
		acl:         acl,
		asyncEvents: asyncEventTypes,
		asyncPool:   defaultAsyncPool,
		ctx:         ctx,
		cancel:      cancel,
	}
	eb.asyncCh = make(chan asyncPayload, defaultAsyncBuffer)
	eb.startAsyncWorkers()
	return eb
}

// startAsyncWorkers 启动副作用层 goroutine pool（R-349）。
func (eb *EventBus) startAsyncWorkers() {
	for i := 0; i < eb.asyncPool; i++ {
		eb.wg.Add(1)
		go eb.asyncWorker(i)
	}
}

// asyncWorker 是副作用层 worker goroutine。
func (eb *EventBus) asyncWorker(id int) {
	defer eb.wg.Done()
	for {
		select {
		case <-eb.ctx.Done():
			return
		case payload := <-eb.asyncCh:
			for _, sub := range payload.subs {
				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("[EventBus] async worker %d: handler panic for event %s: %v",
								id, payload.evt.Type, r)
						}
					}()
					if err := sub.handler(payload.evt); err != nil {
						log.Printf("[EventBus] async worker %d: handler error for event %s: %v",
							id, payload.evt.Type, err)
					}
				}()
			}
		}
	}
}

// Subscribe 注册一个事件处理器。handler 在核心层同步执行。
func (eb *EventBus) Subscribe(eventType string, handler func(events.Event) error) SubscriptionID {
	return eb.SubscribeAs(eventType, "", handler)
}

// SubscribeAs 以指定角色注册事件处理器。
func (eb *EventBus) SubscribeAs(eventType string, role string, handler func(events.Event) error) SubscriptionID {
	return eb.subscribeFiltered(eventType, role, "", false, handler)
}

// SubscribeForGoal 注册 per-goal 过滤的事件处理器（v0.1.0）。
func (eb *EventBus) SubscribeForGoal(goalID string, eventType string, handler func(events.Event) error) SubscriptionID {
	return eb.subscribeFiltered(eventType, "", goalID, false, handler)
}

// SubscribeAsync 注册异步事件处理器（v0.1.0 R-349）。
// handler 在副作用层 goroutine pool 中执行。适用于 I/O 操作（审计写入、通知推送等）。
// 异步 handler 的 panic/error 不影响核心状态转换。
func (eb *EventBus) SubscribeAsync(eventType string, handler func(events.Event) error) SubscriptionID {
	return eb.subscribeFiltered(eventType, "", "", true, handler)
}

// subscribeFiltered 是 Subscribe 的底层实现。
func (eb *EventBus) subscribeFiltered(eventType string, role string, goalID string, async bool, handler func(events.Event) error) SubscriptionID {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	eb.nextID++
	sub := subscription{
		id:      eb.nextID,
		handler: handler,
		role:    role,
		goalID:  goalID,
		async:   async,
	}
	eb.subs[eventType] = append(eb.subs[eventType], sub)
	return eb.nextID
}

// Unsubscribe 取消一个订阅。
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

// Publish 分发事件。核心状态事件同步阻塞分发。副作用事件异步投递（R-349）。
func (eb *EventBus) Publish(evt events.Event) {
	eb.mu.RLock()
	subs := eb.subs[evt.Type]
	allowedRoles := eb.acl[evt.Type]
	eb.mu.RUnlock()

	// 分离同步和异步订阅者
	var syncSubs, asyncSubs []subscription
	for _, sub := range subs {
		if sub.goalID != "" && sub.goalID != evt.GoalID {
			continue
		}
		if allowedRoles != nil && !contains(allowedRoles, sub.role) {
			continue
		}
		if sub.async {
			asyncSubs = append(asyncSubs, sub)
		} else {
			syncSubs = append(syncSubs, sub)
		}
	}

	// 核心层：同步分发（< 1ms handler，禁止 I/O）
	for _, sub := range syncSubs {
		eb.dispatchSync(evt, sub)
	}

	// 副作用层：异步投递（R-349）
	if len(asyncSubs) > 0 {
		eb.dispatchAsync(evt, asyncSubs)
	}
}

// dispatchSync 同步执行单个 handler，带 panic recovery。
func (eb *EventBus) dispatchSync(evt events.Event, sub subscription) {
	func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[EventBus] sync handler panic for event %s: %v", evt.Type, r)
			}
		}()
		if err := sub.handler(evt); err != nil {
			log.Printf("[EventBus] sync handler error for event %s: %v", evt.Type, err)
		}
	}()
}

// dispatchAsync 将事件投递到副作用层 goroutine pool（R-349）。
// 非阻塞：channel 满时丢弃事件并记录警告。
func (eb *EventBus) dispatchAsync(evt events.Event, subs []subscription) {
	payload := asyncPayload{evt: evt, subs: subs}
	select {
	case eb.asyncCh <- payload:
		// 投递成功
	default:
		// channel 满——丢弃非关键事件
		log.Printf("[EventBus] async channel full, dropping event: %s", evt.Type)
	}
}

// Shutdown 优雅关闭 EventBus。等待所有异步 workers 完成。
func (eb *EventBus) Shutdown() {
	eb.cancel()
	eb.wg.Wait()
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
	return fmt.Sprintf("EventBus{subs:%d asyncPool:%d asyncBuf:%d}", len(eb.subs), eb.asyncPool, len(eb.asyncCh))
}
