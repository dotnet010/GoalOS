// Package safemap — 框架基础设施 F1
// Week 0 contract_test: Beck 编写（R-571 测试先行）
// MUST: T-SM-1(ConcurrentReadWrite), T-SM-2(RangeConsistency)
// R-761: Range 基于快照遍历——回调中 Store/Delete 不会死锁

package safemap

import (
	"sync"
	"testing"
)

// TestSafeMap_ConcurrentReadWrite 验证并发读写不 data race
// MUST: 多个 goroutine 同时 Load+Store 不 panic，race detector 不告警
func TestSafeMap_ConcurrentReadWrite(t *testing.T) {
	sm := New[string, int]()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			sm.Store("k", i)
		}(i)
		go func(i int) {
			defer wg.Done()
			sm.Load("k")
		}(i)
	}
	wg.Wait()
	// 不 crash + race detector 不告警 = 通过
}

// TestSafeMap_RangeConsistency 验证 Range 期间并发 Store 不 panic
// MUST: Range 回调中 Store 不导致死锁（快照遍历）
// MUST_NOT: Range 不因并发 Store 而 panic
func TestSafeMap_RangeConsistency(t *testing.T) {
	sm := New[int, string]()
	for i := 0; i < 50; i++ {
		sm.Store(i, "value")
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine 1: 遍历 Range——回调中尝试 Store（模拟"遍历发现过期删除"）
	go func() {
		defer wg.Done()
		sm.Range(func(k int, v string) bool {
			// R-761: 回调中 Store 不会死锁（快照语义）
			sm.Store(k+100, "new")
			return true
		})
	}()

	// Goroutine 2: 并发 Store——模拟其他 goroutine 写入
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			sm.Store(i+200, "concurrent")
		}
	}()

	wg.Wait()
	// 不 deadlock = 通过（R-761 快照方案保证）
}

// TestSafeMap_RangeSnapshotIsolation 验证 Range 快照语义
// MUST: Range 快照不包含 Range 期间 Store 的新值
func TestSafeMap_RangeSnapshotIsolation(t *testing.T) {
	sm := New[int, string]()
	sm.Store(1, "one")
	sm.Store(2, "two")

	seen := make(map[int]bool)
	sm.Range(func(k int, v string) bool {
		seen[k] = true
		// 在回调中 Store 新 key
		sm.Store(3, "three")
		return true
	})

	// 快照语义：Range 期间 Store 的 "three" 不应出现在 seen 中
	if seen[3] {
		t.Error("Range 快照应不包含回调中 Store 的新值——R-761 快照语义违反")
	}
	if !sm.mustExist(1) || !sm.mustExist(2) {
		t.Error("Range 快照应包含 Range 前已存在的 key")
	}
	if !sm.mustExist(3) {
		t.Error("回调中 Store 的值应持久化——仅对本次 Range 不可见")
	}
}

// TestSafeMap_Delete 验证 Delete 基本行为
func TestSafeMap_Delete(t *testing.T) {
	sm := New[string, int]()
	sm.Store("a", 1)
	if _, ok := sm.Load("a"); !ok {
		t.Fatal("Store 后 Load 应返回 true")
	}
	sm.Delete("a")
	if _, ok := sm.Load("a"); ok {
		t.Error("Delete 后 Load 应返回 false")
	}
}

// TestSafeMap_RangeEmpty 验证空 map 的 Range
func TestSafeMap_RangeEmpty(t *testing.T) {
	sm := New[string, int]()
	called := false
	sm.Range(func(k string, v int) bool {
		called = true
		return true
	})
	if called {
		t.Error("空 SafeMap 的 Range 不应执行回调")
	}
}

// BenchmarkSafeMap_Load_Parallel L5 基准——读性能（Linus: P99 < 1μs）
func BenchmarkSafeMap_Load_Parallel(b *testing.B) {
	sm := New[string, int]()
	sm.Store("key", 42)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			sm.Load("key")
		}
	})
}

// BenchmarkSafeMap_Store_Parallel L5 基准——写性能
func BenchmarkSafeMap_Store_Parallel(b *testing.B) {
	sm := New[int, int]()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			sm.Store(i, i)
			i++
		}
	})
}
