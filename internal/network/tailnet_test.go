// tailnet_test.go——tailnet peer 校验契约测试（R-1650 v3/v4——顾问二轮 checklist
// (1)(2)(4) 落地；(3) MagicDNS 真机集成=需 tailnet 主机，诚实登记待验）。
package network

import (
	"context"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeQuerier 可注入查询器（计数+可控错误）。
type fakeQuerier struct {
	mu    sync.Mutex // 夹具竞态防护（race 实证：测试主协程改 peers vs 后台刷新协程读——2026-09-10 修复）
	peers []netip.Addr
	err   error
	calls atomic.Int32
}

func (f *fakeQuerier) q(_ context.Context) ([]netip.Addr, error) {
	f.calls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	// 快照拷贝（返回独立副本——调用方持有不共享底层数组）
	out := make([]netip.Addr, len(f.peers))
	copy(out, f.peers)
	return out, f.err
}

// setPeers 变更 peer 集（测试主协程侧——锁内写）。
func (f *fakeQuerier) setPeers(ips []netip.Addr) {
	f.mu.Lock()
	f.peers = ips
	f.mu.Unlock()
}

// 直注缓存（同包测试钩子——绕后台协程构造确定态）。
func seedCache(c *TailnetPeerCache, ips []netip.Addr, age time.Duration, alive bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.peers = make(map[netip.Addr]struct{}, len(ips))
	for _, ip := range ips {
		c.peers[ip] = struct{}{}
	}
	c.fetchedAt = time.Now().Add(-age)
	c.alive = alive
}

// 顾问 checklist(1)：tailscaled 缺席/查询失败→CGNAT 任意地址 100% ZonePublic。
func TestTailnet_Absent_FailClosed(t *testing.T) {
	c := NewTailnetPeerCache((&fakeQuerier{err: ErrTailscaleAbsent}).q)
	cls := &Classifier{Peers: c}
	for _, s := range []string{"100.64.1.1", "100.100.100.100", "100.127.255.254"} {
		ip := netip.MustParseAddr(s)
		if cls.Classify(ip) != ZonePublic {
			t.Fatalf("tailscaled 缺席时 %s 应 ZonePublic，实得 %s", s, cls.Classify(ip))
		}
		if cls.IsTailnetExempt(ip, 443) {
			t.Fatalf("tailscaled 缺席时 %s 不应免除", s)
		}
	}
	// Quad100 DNS 例外同样 fail-closed（tailscaled 不在线=无 MagicDNS 事实）
	if cls.IsTailnetExempt(netip.MustParseAddr("100.100.100.100"), 53) {
		t.Fatal("tailscaled 缺席时 Quad100:53 不应免除（无 MagicDNS 事实锚）")
	}
}

// 顾问 checklist(2)：已知 peer 精确匹配→ZoneLAN；非 peer CGNAT（如 100.64.1.1）→ZonePublic。
func TestTailnet_PeerGranularity(t *testing.T) {
	peer := netip.MustParseAddr("100.64.7.7")
	c := NewTailnetPeerCache(nil)
	seedCache(c, []netip.Addr{peer}, 0, true)
	cls := &Classifier{Peers: c}
	if got := cls.Classify(peer); got != ZoneLAN {
		t.Fatalf("peer 100.64.7.7 应 ZoneLAN（tailnet 成员），实得 %s", got)
	}
	if !cls.IsTailnetExempt(peer, 443) {
		t.Fatal("peer 任意端口应免除（R-1650 v3(4) Jobs 直批免审批）")
	}
	nonPeer := netip.MustParseAddr("100.64.1.1")
	if got := cls.Classify(nonPeer); got != ZonePublic {
		t.Fatalf("非 peer CGNAT 100.64.1.1 应 ZonePublic（ISP 设施面），实得 %s", got)
	}
	if cls.IsTailnetExempt(nonPeer, 443) {
		t.Fatal("非 peer 不应免除")
	}
	// Quad100 非 peer——不被成员面免除（保留地址天然排除=peer 列表法纪律）
	if cls.IsTailnetExempt(netip.MustParseAddr("100.100.100.100"), 8443) {
		t.Fatal("Quad100 非 53 端口不应免除")
	}
	// Quad100:53 + tailscaled 在线=MagicDNS 基础设施例外（顾问(1)收窄形态）
	if !cls.IsTailnetExempt(netip.MustParseAddr("100.100.100.100"), 53) {
		t.Fatal("Quad100:53 + tailscaled 在线应免除（MagicDNS 基础设施 DNS 例外）")
	}
}

// 缓存超龄=fail-closed（tailscaled 僵死——顾问(2)(3) TOCTOU 收窄）。
func TestTailnet_StaleCache_FailClosed(t *testing.T) {
	peer := netip.MustParseAddr("100.64.7.7")
	c := NewTailnetPeerCache(nil)
	seedCache(c, []netip.Addr{peer}, 10*time.Second, true) // 超龄（>5s TTL）
	cls := &Classifier{Peers: c}
	if c.IsPeer(peer) {
		t.Fatal("缓存超龄应 fail-closed（IsPeer=false）")
	}
	if cls.Classify(peer) != ZonePublic {
		t.Fatal("超龄缓存下 peer 应回落 ZonePublic")
	}
	if c.Alive() {
		t.Fatal("超龄缓存 Alive 应=false")
	}
}

// 顾问 checklist(4)：决策路径零阻塞——缓存热态后 10k 次判定零查询调用
//（结构证：决策路径=纯缓存读，查询只在后台协程——比 30ms 超时更强的挂起免疫）。
func TestTailnet_DecisionPathNeverQueries(t *testing.T) {
	fq := &fakeQuerier{peers: []netip.Addr{netip.MustParseAddr("100.64.7.7")}}
	c := NewTailnetPeerCache(fq.q)
	seedCache(c, fq.peers, 0, true)
	before := fq.calls.Load()
	for i := 0; i < 10000; i++ {
		c.IsPeer(netip.MustParseAddr("100.64.7.7"))
	}
	if got := fq.calls.Load(); got != before {
		t.Fatalf("决策路径发生 %d 次查询调用（应=0——挂起免疫结构性）", got-before)
	}
}

// 后台刷新实证：Start 后查询真实发生+缓存转新鲜（fake 查询器——不涉及真 tailscaled）。
func TestTailnet_BackgroundRefresh(t *testing.T) {
	fq := &fakeQuerier{peers: []netip.Addr{netip.MustParseAddr("100.64.7.7")}}
	c := NewTailnetPeerCache(fq.q)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.Start(ctx)
	defer c.Stop()
	deadline := time.Now().Add(3 * time.Second)
	for !c.Alive() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if !c.Alive() {
		t.Fatal("后台首刷 3s 内未完成（Alive=false）")
	}
	if fq.calls.Load() == 0 {
		t.Fatal("后台刷新未发起查询")
	}
	if !c.IsPeer(netip.MustParseAddr("100.64.7.7")) {
		t.Fatal("刷新后 peer 应可查")
	}
}

// 真机实证：本机 tailscale CLI 缺席→TailscaleCLIQuery=ErrTailscaleAbsent
//（fail-closed 生产路径的真实环境证据——非 mock）。
func TestTailnet_CLIQueryAbsent_RealMachine(t *testing.T) {
	// 本机（Win11 26200）实测 tailscale CLI 不在 PATH 且安装路径缺席——
	// 若运行环境恰好装有 tailscale，本测试自动降级为「查询成功或失败均可」形态断言。
	_, err := TailscaleCLIQuery(context.Background())
	if err != nil && err != ErrTailscaleAbsent {
		t.Fatalf("CLI 查询失败应归一 ErrTailscaleAbsent（fail-closed 语义），实得: %v", err)
	}
	t.Logf("CLI 查询结果: err=%v（缺席=fail-closed 实锤；在场=查询可达）", err)
}

// TestTailnet_EventSeam 事件驱动接缝（会议 #280——顾问二轮(3)收窄落地）：
// (1)peer 集变更=Version 跳变（轮询内哈希比对=变更事件消费面）；
// (2)Invalidate=立即 fail-closed（IsPeer=false——外部事件源插拔点）。
func TestTailnet_EventSeam(t *testing.T) {
	p1 := netip.MustParseAddr("100.64.7.7")
	p2 := netip.MustParseAddr("100.64.9.9")
	fq := &fakeQuerier{peers: []netip.Addr{p1}}
	c := NewTailnetPeerCache(fq.q)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.Start(ctx)
	defer c.Stop()
	deadline := time.Now().Add(3 * time.Second)
	for c.Version() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	v1 := c.Version()
	if v1 == 0 {
		t.Fatal("首刷后 Version 应>0（初始 peer 集=一次变更事件）")
	}
	// 变更 peer 集 → 下轮刷新 Version 跳变
	fq.setPeers([]netip.Addr{p1, p2})
	deadline = time.Now().Add(6 * time.Second) // 刷新周期 3s+余量
	for c.Version() == v1 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if c.Version() == v1 {
		t.Fatal("peer 集变更后 Version 未跳变——变更检测失守")
	}
	if !c.IsPeer(p2) {
		t.Fatal("变更后新 peer 应可见")
	}
	// Invalidate=主动失效——立即 fail-closed
	c.Invalidate()
	if c.IsPeer(p1) {
		t.Fatal("Invalidate 后 IsPeer 应=false（立即 fail-closed）")
	}
	if c.Alive() {
		t.Fatal("Invalidate 后 Alive 应=false")
	}
}
