// Package scheduler — v0.2.0 Week 2: Plugin Fan-Out 本地 cache
// R-737: PluginRegistered→EventBus core layer 同步广播→各模块本地 cache。
// 消除共享 PluginRegistry。零锁（单 goroutine 读写）。
package scheduler

// PluginInfo 是本地缓存的 Plugin 信息。
type PluginInfo struct {
	PluginID     string
	PluginName   string
	PluginType   string   // "capability" | "agent" | "channel"
	Version      string
	Capabilities []string
	BinaryPath   string
}

// PluginCache 是 PipelineRunner 的本地 Plugin cache（Fan-Out 模式）。
// 仅由 EventBus core handler goroutine 读写——零锁。
type PluginCache struct {
	plugins map[string]*PluginInfo
}

// NewPluginCache 创建新的 Plugin 本地 cache。
func NewPluginCache() *PluginCache {
	return &PluginCache{
		plugins: make(map[string]*PluginInfo),
	}
}

// OnPluginRegistered 处理 PluginRegistered 事件——更新本地 cache。
func (pc *PluginCache) OnPluginRegistered(p *PluginInfo) {
	pc.plugins[p.PluginName] = p
}

// OnPluginUnregistered 处理 PluginUnregistered 事件——从本地 cache 移除。
func (pc *PluginCache) OnPluginUnregistered(pluginName string) {
	delete(pc.plugins, pluginName)
}

// Lookup 查找 Plugin——零锁。
func (pc *PluginCache) Lookup(name string) (*PluginInfo, bool) {
	p, ok := pc.plugins[name]
	return p, ok
}

// Count 返回已缓存 Plugin 数量。
func (pc *PluginCache) Count() int {
	return len(pc.plugins)
}
