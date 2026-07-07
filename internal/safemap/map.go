// Package safemap — 框架基础设施 F1（Week 0）
// 并发安全泛型 map。内部 sync.RWMutex + map[K]V。
// R-761: Range 基于快照遍历——回调中 Store/Delete 不会死锁。
// CI 强制: 包级裸 map[K]V 声明→check-naked-map.sh 红色。

package safemap

import "sync"

// Map 并发安全的泛型 map。
// 适用场景: 模块内部共享状态（EventBus handlers、Token 撤销表、Goal 状态缓存）。
// 不适用: 跨模块共享——Plugin 列表用 Fan-Out（R-738）。
type Map[K comparable, V any] struct {
	mu sync.RWMutex
	m  map[K]V
}

// New 创建一个新的并发安全 map。
func New[K comparable, V any]() *Map[K, V] {
	return &Map[K, V]{
		m: make(map[K]V),
	}
}

// Load 读取 key 对应的值。返回值和是否存在。
func (sm *Map[K, V]) Load(key K) (V, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	v, ok := sm.m[key]
	return v, ok
}

// Store 写入 key-value。覆盖已有值。
func (sm *Map[K, V]) Store(key K, value V) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.m[key] = value
}

// Delete 删除 key。
func (sm *Map[K, V]) Delete(key K) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.m, key)
}

// Range 遍历所有 key-value。基于快照——锁内复制 key 列表后释放锁，再逐个 Load+回调。
// R-761: 回调中 Store/Delete 不会死锁。修改对本次 Range 不可见（快照语义）。
func (sm *Map[K, V]) Range(fn func(key K, value V) bool) {
	sm.mu.RLock()
	keys := make([]K, 0, len(sm.m))
	for k := range sm.m {
		keys = append(keys, k)
	}
	sm.mu.RUnlock()

	for _, k := range keys {
		// 重新 Load——遍历期间 key 可能已被其他 goroutine 删除
		if v, ok := sm.Load(k); ok {
			if !fn(k, v) {
				break
			}
		}
	}
}

// Len 返回当前元素数量。
func (sm *Map[K, V]) Len() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.m)
}

// mustExist 测试辅助——不对外暴露。用于 contract_test 验证。
func (sm *Map[K, V]) mustExist(key K) bool {
	_, ok := sm.Load(key)
	return ok
}
