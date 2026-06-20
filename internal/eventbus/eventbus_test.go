package eventbus_test

import (
	"sync"
	"testing"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/pkg/events"
)

// TestPublishSubscribe 验证基本的发布/订阅机制。
// 测试先于实现 —— TDD。
func TestPublishSubscribe(t *testing.T) {
	bus := eventbus.New()

	received := make(chan events.Event, 1)
	bus.Subscribe(events.TypeGoalCreated, func(evt events.Event) error {
		received <- evt
		return nil
	})

	evt := events.Event{
		Type:   events.TypeGoalCreated,
		GoalID: "goal_001",
		Seq:    1,
	}
	bus.Publish(evt)

	select {
	case got := <-received:
		if got.Type != events.TypeGoalCreated {
			t.Errorf("expected GoalCreated, got %s", got.Type)
		}
		if got.GoalID != "goal_001" {
			t.Errorf("expected goal_001, got %s", got.GoalID)
		}
	default:
		t.Fatal("handler was not called")
	}
}

// TestMultipleSubscribers 验证多个订阅者同时接收事件。
func TestMultipleSubscribers(t *testing.T) {
	bus := eventbus.New()
	var wg sync.WaitGroup
	count := 0
	var mu sync.Mutex

	for i := 0; i < 3; i++ {
		wg.Add(1)
		bus.Subscribe(events.TypeGoalCompleted, func(evt events.Event) error {
			defer wg.Done()
			mu.Lock()
			count++
			mu.Unlock()
			return nil
		})
	}

	bus.Publish(events.Event{Type: events.TypeGoalCompleted, GoalID: "goal_001", Seq: 1})
	wg.Wait()

	if count != 3 {
		t.Errorf("expected 3 handlers called, got %d", count)
	}
}

// TestHandlerError 验证 handler 返回 error 不影响其他 handler。
func TestHandlerError(t *testing.T) {
	bus := eventbus.New()
	called := false

	bus.Subscribe(events.TypeGoalCompleted, func(evt events.Event) error {
		return eventbus.ErrHandlerFailed
	})
	bus.Subscribe(events.TypeGoalCompleted, func(evt events.Event) error {
		called = true
		return nil
	})

	bus.Publish(events.Event{Type: events.TypeGoalCompleted, GoalID: "goal_001", Seq: 1})

	if !called {
		t.Fatal("second handler was not called after first returned error")
	}
}

// TestPanicRecovery 验证 handler panic 不影响其他 handler。
func TestPanicRecovery(t *testing.T) {
	bus := eventbus.New()
	called := false

	bus.Subscribe(events.TypeGoalCompleted, func(evt events.Event) error {
		panic("intentional panic")
	})
	bus.Subscribe(events.TypeGoalCompleted, func(evt events.Event) error {
		called = true
		return nil
	})

	// Must not panic
	bus.Publish(events.Event{Type: events.TypeGoalCompleted, GoalID: "goal_001", Seq: 1})

	if !called {
		t.Fatal("second handler was not called after first panicked")
	}
}

// TestUnsubscribe 验证取消订阅后不再接收事件。
func TestUnsubscribe(t *testing.T) {
	bus := eventbus.New()
	received := make(chan events.Event, 1)

	id := bus.Subscribe(events.TypeGoalCreated, func(evt events.Event) error {
		received <- evt
		return nil
	})

	bus.Unsubscribe(id)
	bus.Publish(events.Event{Type: events.TypeGoalCreated, GoalID: "goal_001", Seq: 1})

	select {
	case <-received:
		t.Fatal("handler was called after unsubscribe")
	default:
		// Expected — no event received
	}
}

// TestAllowedSubscriberOnly 验证 allowed-subscriber 列表生效。
// 不在列表中的订阅者收不到事件。
func TestAllowedSubscriberOnly(t *testing.T) {
	// Create bus with explicit allowed subscribers for ActionScheduled
	bus := eventbus.NewWithACL(map[string][]string{
		events.TypeActionScheduled: {"governance", "audit"},
	})

	governanceCalled := false
	schedulerCalled := false

	bus.SubscribeAs(events.TypeActionScheduled, "governance", func(evt events.Event) error {
		governanceCalled = true
		return nil
	})
	bus.SubscribeAs(events.TypeActionScheduled, "scheduler", func(evt events.Event) error {
		schedulerCalled = true
		return nil
	})

	bus.Publish(events.Event{Type: events.TypeActionScheduled, GoalID: "goal_001", Seq: 1})

	if !governanceCalled {
		t.Fatal("governance (allowed) was not called")
	}
	if schedulerCalled {
		t.Fatal("scheduler (not allowed) was called — ACL violation")
	}
}
