// zone.go——网域分类器（D-2 蓝图落地——R-1643 会议 #258 适配裁定）。
// 网域二维治理模型：网络出口策略=治理面问题（非隔离边界类型——R-1506 同构）。
// 三值语义（Kees 安全修正——R-1643②）：免除 data_sharing 审查=仅 loopback；
// RFC1918 LAN 默认仍审查（trust_lan=true 显式免除——信任=管理员显式登记哲学）。
// 实证边界：SBPL 无 CIDR/裸 IP 粒度（仅端口形态可用）——网域粒度=用户态本分类器收口，
// OS 层=端口级放行——三层各司其职（会议 #258 裁决④）。
package network

import "net/netip"

// Zone 网域三值（语义权威=本文件；蓝图 ZoneLocal=ZoneLoopback∪ZoneLAN）。
type Zone string

const (
	// ZoneLoopback 本机回环——data_sharing 审查免除（数据物理不出本机）。
	ZoneLoopback Zone = "LOOPBACK"
	// ZoneLAN 局域网（RFC1918/ULA/链路本地）——默认仍触发 data_sharing 审查
	// （LAN 可经网关代理出境——Kees 修正）；trust_lan=true 显式免除。
	ZoneLAN Zone = "LAN"
	// ZonePublic 公共互联网——严格触发 data_sharing 审查。
	ZonePublic Zone = "PUBLIC"
)

// 预编译 CIDR 规则（IPv4+IPv6 双栈——蓝图 §2.1 全量）。
var (
	loopbackCIDRs = []netip.Prefix{
		netip.MustParsePrefix("127.0.0.0/8"), // IPv4 Loopback
		netip.MustParsePrefix("::1/128"),     // IPv6 Loopback
	}
	lanCIDRs = []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),     // RFC 1918 Class A
		netip.MustParsePrefix("172.16.0.0/12"),  // RFC 1918 Class B
		netip.MustParsePrefix("192.168.0.0/16"), // RFC 1918 Class C
		netip.MustParsePrefix("169.254.0.0/16"), // IPv4 Link-local
		netip.MustParsePrefix("fc00::/7"),       // IPv6 ULA（RFC 4193）
		netip.MustParsePrefix("fe80::/10"),      // IPv6 Link-local
	}
)

// ClassifyIP 网域分类（MUST 契约）：
// ①输入必须合法 netip.Addr（调用方解析——域名解析归底层）；
// ②先 Unmap（IPv4-mapped IPv6 脏数据清洗——::ffff:192.168.1.1 绕过阻断）；
// ③loopback→ZoneLoopback；LAN 族→ZoneLAN；其余→ZonePublic。
func ClassifyIP(ip netip.Addr) Zone {
	ip = ip.Unmap() // [MUST] 阻断 IPv4-mapped IPv6 绕过
	for _, cidr := range loopbackCIDRs {
		if cidr.Contains(ip) {
			return ZoneLoopback
		}
	}
	for _, cidr := range lanCIDRs {
		if cidr.Contains(ip) {
			return ZoneLAN
		}
	}
	return ZonePublic
}

// ClassifyIPString 字符串形态入口（非法输入=ZonePublic——fail-closed：无法识别按公网对待）。
func ClassifyIPString(s string) Zone {
	ip, err := netip.ParseAddr(s)
	if err != nil {
		return ZonePublic
	}
	return ClassifyIP(ip)
}

// ExemptFromDataSharing data_sharing 审查免除判定（Kees 修正——R-1643②）：
// loopback=恒免除；LAN=仅 trustLAN=true 免除；公网=恒不免除。
func ExemptFromDataSharing(z Zone, trustLAN bool) bool {
	switch z {
	case ZoneLoopback:
		return true
	case ZoneLAN:
		return trustLAN
	default:
		return false
	}
}

// ZoneForDecision 决策表分流输入（行 3L/3P——三值最细粒度：免除判定需要 loopback/LAN 区分
// ——Kees 修正 R-1643②）。档位语义：三者皆不提升 I 级（废除涉网硬绑 I3）。
func ZoneForDecision(z Zone) string {
	switch z {
	case ZoneLoopback:
		return "loopback"
	case ZoneLAN:
		return "lan"
	default:
		return "public"
	}
}
