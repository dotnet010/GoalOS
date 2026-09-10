// zone_dialer.go——网域感知拨号器（R-1643 裁决(4)——D-2 蓝图 §2.4 落地）。
// 两条防线合一：(1)DNS 重绑定防御——解析出 IP 后经网域分类校验，**直连该 IP**
// （绝不把域名交回 HTTP Client 二次解析）；(2)force_public_zone 穿透防御——
// 内网代理端点（Clash/vLLM 网关族）配此开关=按公网标记（网域审计语义不失真）。
// 网域粒度=用户态收口（SBPL 无 CIDR 粒度实证——会议 #258）；本层职责=标记+直防，
// 审批门归治理层（data_sharing 排队——行 3P）。
package llm

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/goalos/goalos/internal/network"
)

// ZoneMark 拨号网域标记（一次拨号的网域判定留痕）。
type ZoneMark struct {
	Host      string       // 原始主机名/IP
	IP        string       // 实际连接的 IP（解析后锁定——DNS 重绑定防御证据）
	Zone      network.Zone // 分类结果（force_public_zone 覆盖后）
	RawZone   network.Zone // 原始分类（覆盖前——审计可见覆盖发生）
	ForcedPublic bool      // force_public_zone 覆盖生效
	// TailnetPeer=true=该 IP 经 tailnet peer 实锤（R-1650——审计可见成员身份依据，
	// 区别于静态网段归类）；TailnetExempt=免除 data_sharing 审查（peer 或
	// Quad100:53 MagicDNS 基础设施——IsTailnetExempt 谓词快照）。
	TailnetPeer   bool
	TailnetExempt bool
}

// ZoneDialer 网域感知拨号器（forcePublicZone=配置覆盖——daemon.yaml force_public_zone）。
type ZoneDialer struct {
	ForcePublicZone bool
	OnMark          func(ZoneMark) // 留痕回调（nil=仅日志纪律由调用方）
	// Classifier=tailnet peer 感知层（R-1650——nil=纯静态表，CGNAT 全 ZonePublic
	// fail-closed 基态）。
	Classifier *network.Classifier
	dialer     *net.Dialer
	resolver   *net.Resolver
}

// NewZoneDialer 构造（默认系统解析器+5s 超时）。
func NewZoneDialer(forcePublicZone bool, onMark func(ZoneMark)) *ZoneDialer {
	return &ZoneDialer{
		ForcePublicZone: forcePublicZone,
		OnMark:          onMark,
		dialer:          &net.Dialer{Timeout: 30 * time.Second},
		resolver:        net.DefaultResolver,
	}
}

// DialContext 网域感知拨号（http.Transport.DialContext 挂接形态）：
// (1)host 为 IP 字面量→直接分类；(2)域名→解析（一次）→逐 IP 分类取首个；(3)force_public_zone
// 覆盖（LAN/loopback→public 标记）；(4)**直连锁定 IP**（不二次解析——重绑定防御）；
// (5)解析失败/无可用 IP=fail-closed 拒绝连接。
func (z *ZoneDialer) DialContext(ctx context.Context, network_, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("zone dialer: 地址形态非法 %q: %w", addr, err)
	}

	var ips []netip.Addr
	if literal, perr := netip.ParseAddr(host); perr == nil {
		ips = []netip.Addr{literal}
	} else {
		// 域名解析（一次——结果锁定，连接不重新解析）
		resolved, rerr := z.resolver.LookupIP(ctx, "ip", host)
		if rerr != nil {
			return nil, fmt.Errorf("zone dialer: DNS 解析失败 %q: %w", host, rerr)
		}
		for _, nip := range resolved {
			if a, ok := netip.AddrFromSlice(nip); ok {
				ips = append(ips, a)
			}
		}
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("zone dialer: %q 无可用 IP（fail-closed）", host)
	}

	ip := ips[0]
	rawZone := network.ClassifyIP(ip)
	zone := rawZone
	// R-1650 tailnet peer 实锤重分类（CGNAT 段内 peer→ZoneLAN；其余原样）
	tailnetPeer := false
	if z.Classifier != nil {
		before := zone
		zone = z.Classifier.Classify(ip)
		tailnetPeer = zone != before && zone == network.ZoneLAN
	}
	portNum := 0
	fmt.Sscanf(port, "%d", &portNum)
	tailnetExempt := z.Classifier != nil && z.Classifier.IsTailnetExempt(ip, portNum)
	forced := false
	if z.ForcePublicZone && zone != network.ZonePublic {
		zone = network.ZonePublic // 内网代理端点穿透防御——按公网标记（审计语义不失真）
		forced = true
	}
	if z.OnMark != nil {
		z.OnMark(ZoneMark{Host: host, IP: ip.String(), Zone: zone, RawZone: rawZone, ForcedPublic: forced,
			TailnetPeer: tailnetPeer, TailnetExempt: tailnetExempt})
	}

	// 直连锁定 IP（DNS 重绑定防御——解析与连接同一 IP，不经过二次解析）
	conn, err := z.dialer.DialContext(ctx, network_, net.JoinHostPort(ip.String(), port))
	if err != nil {
		return nil, fmt.Errorf("zone dialer: 连接 %s(%s):%s 失败: %w", host, ip, port, err)
	}
	return conn, nil
}
