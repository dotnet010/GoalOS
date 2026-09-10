//go:build darwin && platformtest

// transport_dir_darwin_test.go——FD3 darwin sun_path 预算决算契约测试（darwin 独占——
// 决算面=darwin 独有，定义面=使用面构建上下文对齐）。
//
// 车道（R-1695-2「分层纳管」）：平台专项特测（build tag `platformtest`）——本文件即
// 「sun_path 预算+目录权限+镜像连带清理」的物理路径边界实测本体（darwin 专项）。
// 触发面：`make test-platform`（darwin 本地）/ darwin-nightly（darwin 平台 CI）。
// RED→GREEN 对存证=
// red-evidence/2026-09-10-fd3-darwin-sunpath-{red,green}.txt：
//
//	D-L1：超预算深目录 Listen 成功+Name()≤103B+落短镜像（/tmp/goalos-fd3- 前缀）；
//	D-L2：镜像内全双工帧往返（中继能力不因迁址退化）；
//	D-L3：Close 连带清理镜像目录（零残留——0700 属主目录不留存量攻击面）；
//	D-L4：预算内短目录直通不迁址（零行为差）。
package fd3

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFD3_DarwinLongDirMirror D-L1/D-L2/D-L3：深目录（确定性超 sun_path 预算）→
// 短镜像承载+帧往返实证+连带清理。
func TestFD3_DarwinLongDirMirror(t *testing.T) {
	// 确定性超预算：t.TempDir() 基底 ~95B 再垫 80B 子目录（跨机名长差免疫）
	deep := filepath.Join(t.TempDir(), strings.Repeat("d", 80))
	if err := os.Mkdir(deep, 0700); err != nil {
		t.Fatalf("深目录构造失败: %v", err)
	}
	ln, err := Listen(deep, "FD3-LongDir")
	if err != nil {
		t.Fatalf("D-L1：超预算深目录 Listen 应经短镜像成功，实际失败: %v", err)
	}
	// D-L1a：Name()≤预算（bind 成功≠路径合规——显式数字断言防隐性截断）
	if got := len(ln.Name()); got > unixSockPathBudget {
		t.Fatalf("D-L1：Name() 长 %d 超预算 %d: %q", got, unixSockPathBudget, ln.Name())
	}
	// D-L1b：落短镜像（/tmp/goalos-fd3- 前缀——非调用方面）
	if !strings.HasPrefix(ln.Name(), "/tmp/goalos-fd3-") {
		t.Fatalf("D-L1：超预算时应落短镜像目录（/tmp/goalos-fd3- 前缀），实际: %q", ln.Name())
	}
	// D-L2：镜像内全双工帧往返（服务端回显——中继能力断言）
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		if f, err := conn.ReadFrame(); err == nil && f.Op == OpData {
			_ = conn.WriteFrame(Frame{Op: OpData, Payload: f.Payload})
		}
	}()
	conn, err := Dial(ln.Name())
	if err != nil {
		t.Fatalf("D-L2：镜像路径 Dial 失败: %v", err)
	}
	defer conn.Close()
	if err := conn.WriteFrame(Frame{Op: OpData, Payload: []byte("mirror-ping")}); err != nil {
		t.Fatalf("D-L2：写帧失败: %v", err)
	}
	f, err := conn.ReadFrame()
	if err != nil || string(f.Payload) != "mirror-ping" {
		t.Fatalf("D-L2：回显不符（迁址退化）——payload=%q err=%v", f.Payload, err)
	}
	// D-L3：Close 连带清理镜像目录（零残留）
	mirrorDir := filepath.Dir(ln.Name())
	if err := ln.Close(); err != nil {
		t.Fatalf("D-L3：Close 失败: %v", err)
	}
	if _, err := os.Stat(mirrorDir); !os.IsNotExist(err) {
		t.Fatalf("D-L3：镜像目录应连带清理（零残留），实际仍存在: %q", mirrorDir)
	}
}

// TestFD3_DarwinShortDirPassthrough D-L4：预算内短目录=直通不迁址（决算零行为差）。
func TestFD3_DarwinShortDirPassthrough(t *testing.T) {
	// 短目录模拟生产浅 tmpDir（macOS t.TempDir() 自身必超预算——不能作短目录夹具）
	short, err := os.MkdirTemp("/tmp", "fd3-short-")
	if err != nil {
		t.Fatalf("短目录构造失败: %v", err)
	}
	defer os.Remove(short)
	ln, err := Listen(short, "FD3-Short")
	if err != nil {
		t.Fatalf("D-L4：短目录 Listen 失败: %v", err)
	}
	defer ln.Close()
	// D-L4a：直通=Name() 落调用方面（非镜像前缀）
	if filepath.Dir(ln.Name()) != short {
		t.Fatalf("D-L4：预算内应直通调用方面 %q，实际落 %q", short, ln.Name())
	}
	// D-L4b：socket 文件实在（bind 物化断言）
	if _, err := os.Stat(ln.Name()); err != nil {
		t.Fatalf("D-L4：socket 文件未物化: %v", err)
	}
	// D-L4c：权限=0600（收窄纪律不因直通/镜像分野漂移）
	if fi, err := os.Stat(ln.Name()); err != nil || fi.Mode().Perm() != 0600 {
		t.Fatalf("D-L4：socket 权限应=0600，实际: %v", err)
	}
}
