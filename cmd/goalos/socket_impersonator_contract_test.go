// UDS 治理通道 + CLI 反验 daemon 身份契约（R-1378 / R-1322）。
//
// 契约（resolutions.yaml R-1378）:
//   CLI 连接 UDS 后必须反验 daemon 身份——对端可执行文件 = goalos-daemon
//   （SO_PEERCRED + exe 白名单 + 祖先链三闸准入 R-1284）；
//   全 API 走 UDS、TCP 仅留 /metrics（R-1322）——治理面（/api/approvals 族）
//   不得暴露于 TCP。
//
// 先红状态（阶段 3.5 测试先行闸口——用例先红）:
//   UDS 通道（~/.goalos/run/daemon.sock）当前未实现——治理面仍挂 TCP mux。
//   红锚 ②: TCP 上 GET /api/approvals 当前返回 200 → 断言 404 或连接拒绝 → 红。
//   探针 ①: config/daemon 包无 socket 路径公开配置（反射枚举公开字段）→
//            辅助信息，不承担红锚。
//
// 转绿任务: 7.20（C-2 表）——UDS 治理通道落地（R-1378）后治理面移出 TCP，本测试转绿。
//
// 断言方式: 行为断言（真实 buildHTTPMux + TCP 服务器）+ 反射探针（公开配置枚举）。
// 禁止读源码文本断言。
package main

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/goalos/goalos/internal/config"
	"github.com/goalos/goalos/internal/daemon"
	"github.com/goalos/goalos/internal/eventbus"
)

// newSocketContractMux 用真实 buildHTTPMux 构造路由表（与 main() 完全同源）。
func newSocketContractMux(t *testing.T) *http.ServeMux {
	t.Helper()
	api := daemon.NewHandler()
	sse := daemon.NewSSEManager()
	cfg := config.Default()
	bus := eventbus.New()
	// missionEng/home 仅被 /api/system/reload 闭包使用，本测试不触发该路由。
	return buildHTTPMux(api, sse, cfg, bus, nil, "")
}

// TestSocket_ImpersonatorRejected — R-1378 核心契约: 治理面不得暴露于 TCP。
//
// 红锚 ②: 当前 GET /api/approvals 经 TCP 返回 200 → 断言 404/连接拒绝 → 先红。
// CLI 反验 daemon 身份（对端可执行文件=goalos-daemon）的前提是 CLI↔daemon
// 只走 UDS——TCP 上存在治理面即违约。
func TestSocket_ImpersonatorRejected(t *testing.T) {
	// ── 探针 ①: UDS socket 路径公开配置（辅助探针）──
	// 契约路径权威值 = ~/.goalos/run/daemon.sock（R-1322）。
	probeUDSSocketConfig(t)

	// ── 红锚 ②: TCP 上不得存在治理路由 ──
	mux := newSocketContractMux(t)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/approvals")
	if err != nil {
		// 连接被拒绝——TCP 面已关闭，UDS-only 契约满足（R-1378）
		t.Logf("TCP /api/approvals 连接被拒绝——UDS-only 契约满足（R-1378）")
		return
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotFound:
		// 通过: 路由已从 TCP mux 移除（治理面 UDS-only，R-1322 "TCP 仅留 /metrics"）
		t.Logf("TCP /api/approvals 返回 404——治理面已移出 TCP（R-1378 转绿）")
	case http.StatusOK:
		t.Errorf("治理面仍暴露于 TCP: GET /api/approvals 返回 200——UDS 治理通道未实现（R-1378 先红）；转绿任务 7.20")
	default:
		t.Errorf("TCP 上 /api/approvals 应返回 404 或连接拒绝（UDS-only 契约 R-1378），实际 %d", resp.StatusCode)
	}

	// 审批子路由同样不得暴露于 TCP（/api/approvals/:id/approve 族）
	resp2, err2 := http.Get(srv.URL + "/api/approvals/x/approve")
	if err2 != nil {
		t.Logf("TCP /api/approvals/:id/approve 连接被拒绝——UDS-only 契约满足（R-1378）")
		return
	}
	defer resp2.Body.Close()
	if resp2.StatusCode == http.StatusOK {
		t.Errorf("审批子路由仍暴露于 TCP: GET /api/approvals/:id/approve 返回 200——UDS 治理通道未实现（R-1378 先红）")
	} else if resp2.StatusCode != http.StatusNotFound {
		t.Errorf("TCP 上 /api/approvals/:id/approve 应返回 404 或连接拒绝（UDS-only 契约 R-1378），实际 %d", resp2.StatusCode)
	}
}

// probeUDSSocketConfig 探针 ①: 反射枚举 config/daemon 公开结构字段，
// 寻找 UDS socket 路径配置。
//
//	存在 → 断言值含权威路径片段 "run/daemon.sock"（R-1322）。
//	不存在 → t.Logf 辅助信息: UDS 通道未落地，红锚由 ② 承担。
func probeUDSSocketConfig(t *testing.T) {
	t.Helper()
	found := walkSocketFields(t, reflect.ValueOf(config.Default()), 0)
	found = walkSocketFields(t, reflect.ValueOf(daemon.NewHandler()), 0) || found
	if !found {
		t.Logf("探针①: config/daemon 无 socket 路径公开配置——UDS 通道未实现（R-1378 先红，红锚=②）")
	}
}

// walkSocketFields 递归遍历结构字段，匹配 socket/unix/uds 命名。
// 匹配到的 string 字段值必须含 "run/daemon.sock"（R-1322 权威路径）。
func walkSocketFields(t *testing.T, v reflect.Value, depth int) bool {
	t.Helper()
	if depth > 4 || !v.IsValid() {
		return false
	}
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return false
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return false
	}
	found := false
	vt := v.Type()
	for i := 0; i < vt.NumField(); i++ {
		f := vt.Field(i)
		if f.PkgPath != "" {
			continue // 未导出字段
		}
		name := strings.ToLower(f.Name)
		if strings.Contains(name, "sock") || strings.Contains(name, "unix") || strings.Contains(name, "uds") {
			found = true
			if f.Type.Kind() != reflect.String {
				t.Errorf("探针①: socket 配置字段 %s 必须为 string（路径值），实际 %s", f.Name, f.Type)
				continue
			}
			val := v.Field(i).String()
			if !strings.Contains(val, "run/daemon.sock") {
				t.Errorf("探针①: socket 路径配置 %s=%q 不符合 R-1322 权威路径（~/.goalos/run/daemon.sock）", f.Name, val)
			} else {
				t.Logf("探针①: 发现 socket 路径配置 %s=%s", f.Name, val)
			}
		}
		if walkSocketFields(t, v.Field(i), depth+1) {
			found = true
		}
	}
	return found
}
