package safemap

import "sync"

// SafeMap — 泛型只读包装（R-1461 方案 2——会议 #218 发现 20）。
//
// 契约：只暴露 Get/Range 读方法，不暴露任何写方法——确实需要共享 map 身份的场景
// （range 遍历/传给依赖具体 map 类型的第三方库）的补充；方案 1（包私有化）覆盖不到
// 的场景的自然延伸。
type SafeMap[K comparable, V any] struct {
	mu sync.RWMutex
	m  map[K]V
}

// NewSafeMap — 构造 SafeMap（初始值拷贝——调用方的 map 不被持有）。
func NewSafeMap[K comparable, V any](initial map[K]V) *SafeMap[K, V] {
	m := make(map[K]V, len(initial))
	for k, v := range initial {
		m[k] = v
	}
	return &SafeMap[K, V]{m: m}
}

// Get — 查询（读锁）。
func (s *SafeMap[K, V]) Get(k K) (V, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.m[k]
	return v, ok
}

// Range — 遍历（R-761 快照语义：锁内复制 key 列表后释放锁，再逐个执行回调）。
// 回调中可调用 Get（重新获取最新值）；回调中调用写方法不存在（SafeMap 无写方法）。
func (s *SafeMap[K, V]) Range(fn func(K, V) bool) {
	s.mu.RLock()
	keys := make([]K, 0, len(s.m))
	for k := range s.m {
		keys = append(keys, k)
	}
	s.mu.RUnlock()
	for _, k := range keys {
		s.mu.RLock()
		v, ok := s.m[k]
		s.mu.RUnlock()
		if !ok {
			continue
		}
		if !fn(k, v) {
			return
		}
	}
}

// Len — 长度（读锁）。
func (s *SafeMap[K, V]) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.m)
}
