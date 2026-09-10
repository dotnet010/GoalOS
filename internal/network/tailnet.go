// tailnet.go——tailnet peer 成员校验（R-1650 v3 语义落地/v4 路线——会议 #271 续）。
// 裁决语义（#266 Jobs 直拍）：100.64.0.0/10 不做区间匹配——只认「当前 tailnet
// 真实存在的 peer」；查不到（无 Tailscale/daemon 僵死/缓存超龄）=ZonePublic
// fail-closed；Quad100（100.100.100.100）等保留地址天然被 peer 列表法排除。
//
// v4 路线（实测修正 v3「ipnstate 结构化包优先」——vendor 实测 5.0MB/442 文件仅
// 类型定义，背离单二进制零依赖第一性）：查询=`tailscale status --json` CLI
// 子进程（全平台通吃零依赖）；**后台刷新协程**承载查询（3s 周期/2s 硬超时），
// 分类决策路径只读缓存=µs 级零阻塞——结构性满足「分类器严禁挂起」（比 30ms
// 同步超时更强：决策路径根本不存在等待）。缓存超龄（>5s 未刷新成功=tailscaled
// 僵死/缺席）= fail-closed。
package network

import (
	"context"
	"encoding/json"
	"net/netip"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// tailnetFreshTTL 缓存新鲜度上限（顾问(2)(3)落点：超龄=fail-closed——TOCTOU 窗口
// 收窄至秒级；事件驱动失效通知=后续优化面，首版轮询纪律）。
const tailnetFreshTTL = 5 * time.Second

// tailnetRefreshEvery 后台刷新周期（<TTL——稳态下缓存恒新鲜）。
const tailnetRefreshEvery = 3 * time.Second

// tailnetQueryTimeout 单次查询硬超时（仅作用于后台协程——决策路径零阻塞）。
const tailnetQueryTimeout = 2 * time.Second

// TailnetQuerier peer 查询器（可注入——生产=TailscaleCLIQuery；测试=fake）。
type TailnetQuerier func(ctx context.Context) ([]netip.Addr, error)

// TailnetPeerCache tailnet peer 缓存（后台刷新+超龄 fail-closed+事件失效接缝）。
//
// 事件驱动面（会议 #280——顾问二轮(3)落地方向收窄实证）：CLI 路线（Jobs 裁决
// 零依赖主路）无事件流——tailscale CLI 无长轮询订阅面；真事件源=LocalAPI
// ipn.Notify 长轮询=重依赖路线（与本裁决冲突）。诚实折中=**接缝化**：轮询
// 仍是载体（3s/5s TTL 不变），但 (1)peer 集变更检测（哈希比对）触发版本递增
// =消费方可观测变更事件；(2)Invalidate() 主动失效口=外部事件源（未来 OS 网络
// 变化监听/LocalAPI watcher 接入点）零改造插拔。
type TailnetPeerCache struct {
	mu        sync.RWMutex
	peers     map[netip.Addr]struct{}
	fetchedAt time.Time
	alive     bool // 最近一次刷新成功（tailscaled 在线证据——Quad100 DNS 例外锚）
	query     TailnetQuerier
	stopCh    chan struct{}
	stopped   sync.Once
	version   atomic.Uint64 // peer 集变更计数（变更事件=消费方可观测——接缝面）
	peersHash uint64        // 上次刷新的 peer 集哈希（变更检测锚）
}

// NewTailnetPeerCache 构造（query=nil 等价恒缺席——fail-closed 空集）。
func NewTailnetPeerCache(query TailnetQuerier) *TailnetPeerCache {
	if query == nil {
		query = func(context.Context) ([]netip.Addr, error) { return nil, ErrTailscaleAbsent }
	}
	return &TailnetPeerCache{peers: map[netip.Addr]struct{}{}, query: query, stopCh: make(chan struct{})}
}

// Start 后台刷新协程（3s 周期；立即首刷——决策路径从不等待查询）。
func (c *TailnetPeerCache) Start(ctx context.Context) {
	refresh := func() {
		qctx, cancel := context.WithTimeout(ctx, tailnetQueryTimeout)
		ips, err := c.query(qctx)
		cancel()
		c.mu.Lock()
		if err == nil {
			newHash := hashPeers(ips)
			if newHash != c.peersHash {
				c.version.Add(1) // 变更事件（消费方可经 Version() 观测——接缝面）
				c.peersHash = newHash
			}
			c.peers = make(map[netip.Addr]struct{}, len(ips))
			for _, ip := range ips {
				c.peers[ip] = struct{}{}
			}
			c.fetchedAt = time.Now()
			c.alive = true
		} else {
			c.alive = false // 查询失败=tailscaled 僵死/缺席——缓存保留但超龄机制接管 fail-closed
		}
		c.mu.Unlock()
	}
	go refresh()
	go func() {
		t := time.NewTicker(tailnetRefreshEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-c.stopCh:
				return
			case <-t.C:
				refresh()
			}
		}
	}()
}

// Stop 停后台协程（幂等）。
func (c *TailnetPeerCache) Stop() { c.stopped.Do(func() { close(c.stopCh) }) }

// Version peer 集变更计数（事件接缝——变更检测=轮询内哈希比对，真实事件源
// 可叠加调用 Invalidate/或直接观测 Version 跳变）。
func (c *TailnetPeerCache) Version() uint64 { return c.version.Load() }

// Invalidate 主动失效（外部事件源接缝——未来 OS 网络变化监听/LocalAPI watcher
// 接入点；失效=立即超龄化=fresh 判定 false=fail-closed 直到下次成功刷新）。
func (c *TailnetPeerCache) Invalidate() {
	c.mu.Lock()
	c.fetchedAt = time.Time{}
	c.mu.Unlock()
}

// hashPeers peer 集哈希（变更检测锚——FNV 轻量即可，非安全面）。
func hashPeers(ips []netip.Addr) uint64 {
	h := uint64(1469598103934665603) // FNV-1a 64 offset
	for _, ip := range ips {
		for _, b := range ip.As16() {
			h ^= uint64(b)
			h *= 1099511628211
		}
	}
	return h
}

// IsPeer 成员校验（决策热路径——纯缓存读 µs 级；缓存缺席/超龄=false fail-closed）。
func (c *TailnetPeerCache) IsPeer(ip netip.Addr) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.fetchedAt.IsZero() || time.Since(c.fetchedAt) > tailnetFreshTTL {
		return false
	}
	_, ok := c.peers[ip.Unmap()]
	return ok
}

// Alive tailscaled 在线且缓存新鲜（Quad100 MagicDNS 例外的存在性锚——
// tailscaled 不在线时 Quad100 只是普通 CGNAT 地址=ZonePublic）。
func (c *TailnetPeerCache) Alive() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.alive && !c.fetchedAt.IsZero() && time.Since(c.fetchedAt) <= tailnetFreshTTL
}

// ─── CLI 查询器（生产形态——零依赖全平台） ───

// ErrTailscaleAbsent tailscale CLI 缺席/查询失败（fail-closed 信号）。
var ErrTailscaleAbsent = errTailscaleAbsent{}

type errTailscaleAbsent struct{}

func (errTailscaleAbsent) Error() string { return "tailscale CLI 缺席或查询失败" }

// tailscaleStatusJSON `tailscale status --json` 最小解析面（只要 Peer[].TailscaleIPs）。
type tailscaleStatusJSON struct {
	Peer map[string]struct {
		TailscaleIPs []string `json:"TailscaleIPs"`
	} `json:"Peer"`
}

// TailscaleCLIQuery 生产查询器（`tailscale status --json` 子进程——Windows 补
// 安装路径兜底；CLI 缺席/非零退出/JSON 畸形=ErrTailscaleAbsent fail-closed）。
// 注：进程 spawn 成本（Windows ~50ms）仅落后台刷新协程——决策路径零感知。
func TailscaleCLIQuery(ctx context.Context) ([]netip.Addr, error) {
	bin, err := exec.LookPath("tailscale")
	if err != nil && runtime.GOOS == "windows" {
		bin, err = exec.LookPath(`C:\Program Files\Tailscale\tailscale.exe`)
	}
	if err != nil {
		return nil, ErrTailscaleAbsent
	}
	out, err := exec.CommandContext(ctx, bin, "status", "--json").Output()
	if err != nil {
		return nil, ErrTailscaleAbsent
	}
	var st tailscaleStatusJSON
	if err := json.Unmarshal(out, &st); err != nil {
		return nil, ErrTailscaleAbsent
	}
	var ips []netip.Addr
	for _, peer := range st.Peer {
		for _, s := range peer.TailscaleIPs {
			if ip, perr := netip.ParseAddr(s); perr == nil {
				ips = append(ips, ip.Unmap())
			}
		}
	}
	return ips, nil
}
