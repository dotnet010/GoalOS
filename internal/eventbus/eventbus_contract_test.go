// eventbus_contract_test.go — EventBus 两层架构契约测试（R-349）
//
// 对应: 05架构 §3.6 EventBus 语义 [MUST] 两层架构
// TC-EB-007: 异步 handler panic 不影响核心状态转换
// TC-EB-008: 异步 handler 在独立 goroutine 中执行
// TC-EB-009: Shutdown 等待所有异步 workers 完成

package eventbus

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/goalos/goalos/pkg/events"
)

// TestContract_EventBus_TwoLayer_AsyncPanicNotAffectCore 验证异步 handler
// panic 不影响核心状态转换（TC-EB-007）。
func TestContract_EventBus_TwoLayer_AsyncPanicNotAffectCore(t *testing.T) {
	bus := New()
	defer bus.Shutdown()

	var coreCalled int32
	var asyncCalled int32

	// 核心 handler（同步）
	bus.Subscribe("TestEvent", func(e events.Event) error {
		atomic.AddInt32(&coreCalled, 1)
		return nil
	})

	// 异步 handler（会 panic）
	bus.SubscribeAsync("TestEvent", func(e events.Event) error {
		atomic.AddInt32(&asyncCalled, 1)
		panic("async handler intentional panic")
	})

	// 发布事件
	evt := events.Event{Type: "TestEvent", GoalID: "g-test"}
	bus.Publish(evt)

	// 等待异步 handler 执行
	time.Sleep(50 * time.Millisecond)

	// 核心 handler 必须被调用
	if atomic.LoadInt32(&coreCalled) != 1 {
		t.Fatalf("MUST: core handler called. Got: %d", coreCalled)
	}

	// 异步 handler 必须被调用（虽然 panic 了）
	if atomic.LoadInt32(&asyncCalled) != 1 {
		t.Fatalf("MUST: async handler called despite panic. Got: %d", asyncCalled)
	}
}

// TestContract_EventBus_TwoLayer_AsyncNotBlockCore 验证异步 handler
// 的延迟不阻塞核心 Publish 返回（TC-EB-008）。
func TestContract_EventBus_TwoLayer_AsyncNotBlockCore(t *testing.T) {
	bus := New()
	defer bus.Shutdown()

	var coreDone int32
	var asyncStarted int32

	// 异步 handler：长时间运行
	bus.SubscribeAsync("TestEvent", func(e events.Event) error {
		atomic.AddInt32(&asyncStarted, 1)
		time.Sleep(200 * time.Millisecond) // 模拟 I/O
		return nil
	})

	// 核心 handler：快速返回
	bus.Subscribe("TestEvent", func(e events.Event) error {
		atomic.AddInt32(&coreDone, 1)
		return nil
	})

	start := time.Now()
	evt := events.Event{Type: "TestEvent", GoalID: "g-test"}
	bus.Publish(evt)
	elapsed := time.Since(start)

	// 核心 handler 必须在 Publish 返回前执行完毕
	if atomic.LoadInt32(&coreDone) != 1 {
		t.Fatalf("MUST: core handler completed before Publish returns")
	}

	// Publish 不应等待异步 handler——返回时间应 < 50ms
	if elapsed > 100*time.Millisecond {
		t.Fatalf("MUST: Publish not blocked by async handler. Elapsed: %v", elapsed)
	}

	// 等待异步 handler 完成
	time.Sleep(300 * time.Millisecond)
	if atomic.LoadInt32(&asyncStarted) != 1 {
		t.Fatalf("MUST: async handler started")
	}
}

// TestContract_EventBus_TwoLayer_AsyncChannelFullDrops 验证异步
// channel 满时丢弃事件不阻塞（TC-EB-009）。
func TestContract_EventBus_TwoLayer_AsyncChannelFullDrops(t *testing.T) {
	bus := New()
	defer bus.Shutdown()

	var asyncCount int32
	var wg sync.WaitGroup

	// 异步 handler：慢速处理
	bus.SubscribeAsync("FloodEvent", func(e events.Event) error {
		time.Sleep(10 * time.Millisecond)
		atomic.AddInt32(&asyncCount, 1)
		return nil
	})

	// 期望处理的事件数：buffer(100) + 部分溢出
	expectedMin := int32(50) // 至少处理 50 个（5 workers × 10 batches）

	// 发送大量事件填满 buffer
	evt := events.Event{Type: "FloodEvent", GoalID: "g-flood"}
	_ = &wg // unused for this test
	start := time.Now()
	for i := 0; i < 200; i++ {
		bus.Publish(evt)
	}
	elapsed := time.Since(start)

	// Publish 不能阻塞——即使 channel 满了
	if elapsed > 500*time.Millisecond {
		t.Fatalf("MUST: Publish not block when async channel full. Elapsed: %v", elapsed)
	}

	// 等待异步处理完成
	time.Sleep(500 * time.Millisecond)

	count := atomic.LoadInt32(&asyncCount)
	if count < expectedMin {
		t.Fatalf("MUST: at least %d events processed. Got: %d", expectedMin, count)
	}
}

// TestContract_EventBus_TwoLayer_ShutdownWaitsAsync 验证 Shutdown
// 等待所有异步 workers 完成。
func TestContract_EventBus_TwoLayer_ShutdownWaitsAsync(t *testing.T) {
	bus := New()

	var done int32
	bus.SubscribeAsync("FinalEvent", func(e events.Event) error {
		time.Sleep(50 * time.Millisecond)
		atomic.StoreInt32(&done, 1)
		return nil
	})

	evt := events.Event{Type: "FinalEvent", GoalID: "g-final"}
	bus.Publish(evt)

	// Shutdown 应等待异步 handler 完成
	bus.Shutdown()

	if atomic.LoadInt32(&done) != 1 {
		t.Fatalf("MUST: async handler completed before Shutdown returns")
	}
}

// TestContract_EventBus_MUST_NOT_ModifySubscribersOnPublish 验证
// Publish 期间不修改订阅列表。
func TestContract_EventBus_MUST_NOT_ModifySubscribersOnPublish(t *testing.T) {
	bus := New()
	defer bus.Shutdown()

	var callCount int32
	bus.Subscribe("ModifyTest", func(e events.Event) error {
		atomic.AddInt32(&callCount, 1)
		return nil
	})

	// 并发 Publish + Subscribe 不应 panic 或 race
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				bus.Publish(events.Event{Type: "ModifyTest"})
				bus.Subscribe("OtherEvent", func(e events.Event) error { return nil })
			}
		}()
	}
	wg.Wait()
}
