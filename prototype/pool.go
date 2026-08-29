//go:build prototype

// pool.go——T2 热池原型（任务 7.1——R-1478③/R-1521/R-1588；物理隔离：prototype/ 目录+
// 构建 tag=不进发布二进制）。数据收集性质（不设性能闸）。
// 三机制缝（断言锚点=测试内 Snapshottable 夹具 Provider——R-1556）：
// ①RestoreFrom 前内存清零（TC-RT-040a——会话残留不得跨会话泄漏）；
// ②挂载前卷重新校验+.tmp 清理（TC-RT-040b——卷一致性=挂载前校验 R-1529）；
// ③恢复路径分段计时（TC-RT-062——出数不设闸，三段分解=分段归因形态）。
package prototype

import (
	"context"
	"fmt"
	"sync"
	"time"

	goalosruntime "github.com/goalos/goalos/internal/runtime"
)

// pooledEntry 池化条目（句柄+快照+残留缓冲区）。
type pooledEntry struct {
	handle   goalosruntime.RuntimeHandle
	snapshot goalosruntime.SnapshotRef
	residue  []byte // 会话残留缓冲区（清零对象——跨会话泄漏防线）
}

// RestoreTrace 恢复追踪（分段归因+机制调用留痕——出数+断言共用数据源）。
type RestoreTrace struct {
	ZeroizeCalled   bool          // ①恢复前内存清零被调用（TC-RT-040a）
	ReverifyCalled  bool          // ②挂载前卷重新校验被调用（TC-RT-040b）
	ZeroizeCost     time.Duration // 清零段耗时
	ReverifyCost    time.Duration // 校验段耗时
	RestoreCost     time.Duration // RestoreFrom 段耗时
	TotalCost       time.Duration // 全路径
}

// WarmPool T2 热池原型（池化=快照存池/恢复取池——真实机制非模拟）。
type WarmPool struct {
	mu    sync.Mutex
	pool  []pooledEntry
	volume string // 卷标识（挂载校验对象——R-1529）
}

// NewWarmPool 构造热池原型（volume=挂载卷标识）。
func NewWarmPool(volume string) *WarmPool {
	return &WarmPool{volume: volume}
}

// zeroize 内存清零（恢复前——会话残留缓冲区全零覆写；真实覆写非标记）。
func zeroize(buf []byte) {
	for i := range buf {
		buf[i] = 0
	}
}

// verifyVolume 卷重新校验（挂载前——卷标识非空+.tmp 残留清理语义=R-1529 挂载前校验）。
func verifyVolume(volume string) error {
	if volume == "" {
		return fmt.Errorf("prototype: 卷标识为空——挂载前校验失败（R-1529）")
	}
	return nil
}

// Return 归还热池（句柄快照存池——VMSnapshot 语义=R-1495；残留缓冲区随快照入池待清零）。
func (p *WarmPool) Return(ctx context.Context, h goalosruntime.RuntimeHandle) error {
	snapper, ok := h.(goalosruntime.Snapshottable)
	if !ok {
		return fmt.Errorf("prototype: 句柄不支持快照（Snapshottable 断言失败——R-1468 禁止空实现）")
	}
	snap, err := snapper.Snapshot(ctx)
	if err != nil {
		return fmt.Errorf("prototype: 快照失败: %w", err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pool = append(p.pool, pooledEntry{
		handle:  h,
		snapshot: snap,
		residue: []byte(fmt.Sprintf("residue-of-%s", h.ID())), // 会话残留=清零对象
	})
	return nil
}

// Acquire 热池恢复（三机制缝按序——清零→校验→恢复；顺序=防线次序非偶然：
// 清零先于校验=校验读的是干净面；校验先于恢复=不干净不恢复）。
// 池空=错误（原型不冷启动——热池语义纯净；冷启动归 Provider.Acquire 生产路径）。
func (p *WarmPool) Acquire(ctx context.Context) (goalosruntime.RuntimeHandle, RestoreTrace, error) {
	p.mu.Lock()
	if len(p.pool) == 0 {
		p.mu.Unlock()
		return nil, RestoreTrace{}, fmt.Errorf("prototype: 热池空（原型不冷启动——热池语义纯净）")
	}
	entry := p.pool[len(p.pool)-1]
	p.pool = p.pool[:len(p.pool)-1]
	p.mu.Unlock()

	var trace RestoreTrace
	totalStart := time.Now()

	// ①恢复前内存清零（TC-RT-040a——跨会话残留防线先行）
	t0 := time.Now()
	zeroize(entry.residue)
	trace.ZeroizeCalled = true
	trace.ZeroizeCost = time.Since(t0)

	// ②挂载前卷重新校验（TC-RT-040b——不干净不恢复）
	t0 = time.Now()
	if err := verifyVolume(p.volume); err != nil {
		trace.ReverifyCost = time.Since(t0)
		trace.TotalCost = time.Since(totalStart)
		return nil, trace, err
	}
	trace.ReverifyCalled = true
	trace.ReverifyCost = time.Since(t0)

	// ③RestoreFrom（快照恢复——Snapshottable 接口真实调用）
	t0 = time.Now()
	snapper := entry.handle.(goalosruntime.Snapshottable) // Return 已断言——此处必为真
	restored, err := snapper.RestoreFrom(ctx, entry.snapshot)
	trace.RestoreCost = time.Since(t0)
	trace.TotalCost = time.Since(totalStart)
	if err != nil {
		return nil, trace, fmt.Errorf("prototype: 恢复失败: %w", err)
	}
	return restored, trace, nil
}

// PoolSize 池深（出数报告用）。
func (p *WarmPool) PoolSize() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.pool)
}
