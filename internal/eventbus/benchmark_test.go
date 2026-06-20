// EventBus 性能基准测试。
// 设计依据：05 架构文档 §3（P95 < 10ms）、R240。
package eventbus_test

import (
	"testing"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/pkg/events"
)

// BenchmarkPublish 测试单 handler 发布延迟。
func BenchmarkPublish(b *testing.B) {
	bus := eventbus.New()
	bus.Subscribe(events.TypeGoalCompleted, func(evt events.Event) error {
		return nil
	})

	evt := events.Event{Type: events.TypeGoalCompleted, GoalID: "bench", Seq: 1, Source: "bench"}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		bus.Publish(evt)
	}
}

// BenchmarkPublish10Handlers 测试 10 个 handler 的发布延迟。
// 预期：P95 < 10ms（架构 NFR）。
func BenchmarkPublish10Handlers(b *testing.B) {
	bus := eventbus.New()
	for i := 0; i < 10; i++ {
		bus.Subscribe(events.TypeGoalCompleted, func(evt events.Event) error {
			return nil
		})
	}

	evt := events.Event{Type: events.TypeGoalCompleted, GoalID: "bench", Seq: 1, Source: "bench"}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		bus.Publish(evt)
	}
}
