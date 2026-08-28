// registry.go——Provider 注册表（W1 最小形态：注册机制+零注册骨架纪律 R-1468
// 「未实现不注册」——当前注册表为空，AcquireForTier 必然 ErrNoProviderRegistered，
// 该失败即 TC-RT-001 族先红锚点）。
// SPI v2 全量方法集=任务 3.2（接口定稿权威=05 §X.6.5）；本文件的 ProviderHandle
// 仅为注册表身份接口（Name/Tier），非 SPI 定义。
package runtime

import (
	"errors"
	"sync"
)

// ErrNoProviderRegistered 注册表空——骨架纪律（R-1468）下的合法先红状态。
var ErrNoProviderRegistered = errors.New("runtime: 无注册 Provider（骨架纪律 R-1468——未实现不注册）")

// ProviderHandle 注册表身份接口（W1 最小——Name/Tier；SPI v2 全量方法集归任务 3.2）。
type ProviderHandle interface {
	Name() string
	Tier() string
}

// ProviderRegistry Provider 注册表（未实现不注册——静态扫描断言点=TC-RT-090 族）。
type ProviderRegistry struct {
	mu     sync.RWMutex
	byTier map[string][]ProviderHandle
}

// NewProviderRegistry 构造空注册表。
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{byTier: make(map[string][]ProviderHandle)}
}

// Register 注册 Provider（骨架纪律：仅真实实现可注册——空实现注册=违规）。
func (r *ProviderRegistry) Register(p ProviderHandle) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byTier[p.Tier()] = append(r.byTier[p.Tier()], p)
}

// AcquireForTier 按档取 Provider（无注册=ErrNoProviderRegistered——TC-RT-001 族先红锚点）。
func (r *ProviderRegistry) AcquireForTier(tier string) (ProviderHandle, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ps := r.byTier[tier]
	if len(ps) == 0 {
		return nil, ErrNoProviderRegistered
	}
	return ps[0], nil
}
