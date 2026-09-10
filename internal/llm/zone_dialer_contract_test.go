// zone_dialer_contract_test.go——网域感知拨号器契约测试（R-1643 裁决(4)——D-2 蓝图 §2.4）。
// TC-RT-013 穿透侧（force_public_zone 覆盖标记）+TC-RT-101 无感侧（loopback 本地模型
// 静默执行——httptest 形态，会议 #258 适配：LAN Ollama 依赖改 127.0.0.1 监听）。
// 12 清单 F/G 节登记（实现同步补强——非先红标注）。
package llm

import (
	"context"
	"fmt"
	"net"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/goalos/goalos/internal/network"
)

// TestZoneDialer_LoopbackSilent（TC-RT-101 无感侧）：
// 本地监听（127.0.0.1）端点=loopback——连接成功+网域标记 loopback+免除 data_sharing
// （本地模型无感执行语义：不触发任何中断提示）。
func TestZoneDialer_LoopbackSilent(t *testing.T) {
	srv := httptest.NewServer(nil)
	defer srv.Close()

	var marks []ZoneMark
	d := NewZoneDialer(false, func(m ZoneMark) { marks = append(marks, m) })
	host := srv.Listener.Addr().String()
	conn, err := d.DialContext(context.Background(), "tcp", host)
	if err != nil {
		t.Fatalf("loopback 连接必须成功（无感执行）: %v", err)
	}
	_ = conn.Close()
	if len(marks) != 1 {
		t.Fatalf("留痕恰好一次，实际 %d", len(marks))
	}
	m := marks[0]
	if m.Zone != network.ZoneLoopback {
		t.Fatalf("127.0.0.1 应=loopback，实际 %s", m.Zone)
	}
	if m.ForcedPublic {
		t.Fatal("无覆盖时 ForcedPublic 必须=false")
	}
	// DNS 重绑定防御证据：连接的 IP=解析锁定的 IP（127.0.0.1）
	if m.IP != "127.0.0.1" {
		t.Fatalf("连接 IP 应为解析锁定值 127.0.0.1，实际 %s", m.IP)
	}
	// 免除语义（Kees 修正闭环）：loopback 恒免除 data_sharing
	if !network.ExemptFromDataSharing(m.Zone, false) {
		t.Fatal("loopback 必须恒免除 data_sharing 审查")
	}
}

// TestZoneDialer_ForcePublicZone（TC-RT-013 穿透侧）：
// 内网代理端点（LAN 形态）配 force_public_zone=true → 标记=ZonePublic+ForcedPublic=true
// （RawZone 保留原始分类——审计可见覆盖发生）。
func TestZoneDialer_ForcePublicZone(t *testing.T) {
	// 启动 127.0.0.1 监听承载连接（分类目标=LAN 字面量经拨号器解析路径覆盖验证）
	srv := httptest.NewServer(nil)
	defer srv.Close()

	var marks []ZoneMark
	// 直接以 LAN IP 字面量为目标的覆盖验证（无需真实 LAN 服务——分类在拨号前）
	d := NewZoneDialer(true, func(m ZoneMark) { marks = append(marks, m) })
	// LAN 字面量：连接会失败（无服务）但分类标记必须先发生
	_, _ = d.DialContext(context.Background(), "tcp", "192.168.1.100:7890")
	if len(marks) != 1 {
		t.Fatalf("分类标记必须先于连接结果，实际 %d 次", len(marks))
	}
	m := marks[0]
	if m.RawZone != network.ZoneLAN {
		t.Fatalf("192.168.1.100 原始分类应=LAN，实际 %s", m.RawZone)
	}
	if m.Zone != network.ZonePublic || !m.ForcedPublic {
		t.Fatalf("force_public_zone 覆盖后应=public+forced，实际 %s forced=%v", m.Zone, m.ForcedPublic)
	}
	// 覆盖后不免除（穿透防御闭环：data_sharing 审查不可被内网代理形态绕过）
	if network.ExemptFromDataSharing(m.Zone, true) {
		t.Fatal("覆盖为 public 后任何 trust 开关不得免除（穿透防御闭环）")
	}

	// 对照：无覆盖时 LAN 保持 LAN（免除判定归 trust_lan——决策层）
	d2 := NewZoneDialer(false, func(m ZoneMark) { marks = append(marks, m) })
	_, _ = d2.DialContext(context.Background(), "tcp", "192.168.1.100:7890")
	last := marks[len(marks)-1]
	if last.Zone != network.ZoneLAN || last.ForcedPublic {
		t.Fatalf("无覆盖时 LAN 必须保持 LAN 标记，实际 %s forced=%v", last.Zone, last.ForcedPublic)
	}
}

// TestZoneDialer_FailClosed 解析失败=fail-closed 拒绝（不静默放行）——hermetic：
// 同包注入死解析器（Dial 必败）替代外网 DNS 依赖（本机 DNS 可被劫持——不可靠证据）。
// 外加：地址形态非法（缺端口）=拒绝。
func TestZoneDialer_FailClosed(t *testing.T) {
	// 形态非法路径
	d0 := NewZoneDialer(false, nil)
	if _, err := d0.DialContext(context.Background(), "tcp", "bad-addr-no-port"); err == nil {
		t.Fatal("缺端口地址必须拒绝")
	}
	// 解析失败路径（死解析器——确定性）
	d := NewZoneDialer(false, nil)
	d.resolver = &net.Resolver{PreferGo: true, Dial: func(context.Context, string, string) (net.Conn, error) {
		return nil, fmt.Errorf("dns dead by fixture")
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := d.DialContext(ctx, "tcp", "anything.example.com:443")
	if err == nil {
		t.Fatal("解析失败必须拒绝（fail-closed）")
	}
}
