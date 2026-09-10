// zone_contract_test.go——网域分类器契约测试（TC-RT-012——12 清单 F 节登记；R-1643）。
// 断言来源=D-2 蓝图 §四 TC-RT-012+会议 #258 Kees 修正（loopback/LAN/公网三值）。
package network

import (
	"net/netip"
	"testing"
)

// TestNetwork_ZoneClassifier_IPv6Bypass（TC-RT-012）：IPv6 绕过拦截——
// IPv4-mapped（::ffff:192.168.1.1）/ULA（fc00::1）/v6 回环正确归类，格式混淆绕过阻断。
func TestNetwork_ZoneClassifier_IPv6Bypass(t *testing.T) {
	cases := []struct {
		in   string
		want Zone
	}{
		// TC-RT-012 核心：IPv4-mapped IPv6 绕过——Unmap 后命中 LAN
		{"::ffff:192.168.1.1", ZoneLAN},
		{"::ffff:10.0.0.5", ZoneLAN},
		{"::ffff:127.0.0.1", ZoneLoopback}, // mapped loopback→loopback（Unmap 后 127.0.0.1）
		// IPv6 原生
		{"fc00::1", ZoneLAN},      // ULA
		{"fd12:3456::1", ZoneLAN}, // ULA
		{"fe80::1", ZoneLAN},      // link-local
		{"::1", ZoneLoopback},     // v6 回环
		// IPv4 原生
		{"127.0.0.1", ZoneLoopback},
		{"127.0.1.5", ZoneLoopback}, // /8 整段
		{"10.0.1.5", ZoneLAN},       // RFC1918 A（蓝图 TC-RT-101 Ollama 形态）
		{"172.16.8.8", ZoneLAN},     // RFC1918 B
		{"192.168.1.100", ZoneLAN},  // RFC1918 C（蓝图 TC-RT-013 代理形态）
		{"169.254.1.1", ZoneLAN},    // link-local
		// 公网
		{"8.8.8.8", ZonePublic},
		{"1.1.1.1", ZonePublic},
		{"2606:4700:4700::1111", ZonePublic}, // v6 公网
		// 非法输入=fail-closed 公网
		{"not-an-ip", ZonePublic},
		{"", ZonePublic},
	}
	for _, c := range cases {
		ip, err := netip.ParseAddr(c.in)
		var got Zone
		if err != nil {
			got = ClassifyIPString(c.in) // 非法输入路径
		} else {
			got = ClassifyIP(ip)
		}
		if got != c.want {
			t.Fatalf("ClassifyIP(%q)=%s，期望 %s", c.in, got, c.want)
		}
	}
}

// TestNetwork_Zone_ExemptionKeesRule 免除规则（Kees 修正——R-1643-2）：
// loopback 恒免除；LAN 默认不免除、trust_lan=true 免除；公网恒不免除。
func TestNetwork_Zone_ExemptionKeesRule(t *testing.T) {
	if !ExemptFromDataSharing(ZoneLoopback, false) {
		t.Fatal("loopback 必须恒免除 data_sharing")
	}
	if ExemptFromDataSharing(ZoneLAN, false) {
		t.Fatal("LAN 默认不得免除（Kees：LAN 可经网关代理出境）")
	}
	if !ExemptFromDataSharing(ZoneLAN, true) {
		t.Fatal("LAN+trust_lan=true 必须免除（管理员显式登记）")
	}
	if ExemptFromDataSharing(ZonePublic, true) {
		t.Fatal("公网恒不免除")
	}
}
