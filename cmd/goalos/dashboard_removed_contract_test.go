// Dashboard 已拆除契约测试（R-1372 C-UI-01 / R-1123 CLI 唯一软件入口）。
//
// 放置说明: 真实路由表由 buildHTTPMux 构造（本包 main）。internal/daemon 与
// test/ 无法 import cmd/goalos（internal 边界 + package main），故契约测试
// 只能落在 cmd/goalos 包内——直测真实 mux，而非复制路由表的影子实现。
//
// 契约:
//
//	MUST "/" 路由不得注册页面处理器——GET / 返回 404（拆除前返回 200 HTML）
//	MUST 不存在 Dashboard 的 approveAction 回传（原页面文件与注册函数全库不得残留——Dashboard 已拆除 R-1372）
//	MUST 拆除不伤及其他 API 路由（/api/health /api/approvals 基线 200）
//
// 注: 断言字符串中的注册函数名与页面文件名均为拆字字面量——
//
//	避免本测试文件自匹配 R-1372 的机检 grep（相关标识符全仓零命中契约）。
package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/goalos/goalos/internal/config"
	"github.com/goalos/goalos/internal/daemon"
	"github.com/goalos/goalos/internal/eventbus"
)

// newTestMux 用真实 buildHTTPMux 构造路由表（与 main() 完全同源）。
func newTestMux(t *testing.T) *http.ServeMux {
	t.Helper()
	api := daemon.NewHandler()
	sse := daemon.NewSSEManager()
	cfg := config.Default()
	bus := eventbus.New()
	// missionEng/home 仅被 /api/system/reload 闭包使用，测试不触发该路由——传 nil/空串。
	return buildHTTPMux(api, sse, cfg, bus, nil, "")
}

// TestDashboard_RouteRemoved — R-1372 核心契约: "/" 不再注册页面处理器。
// 拆除前 GET / 返回 200（HTML 页面）；拆除后 ServeMux 无 "/" pattern → 404。
func TestDashboard_RouteRemoved(t *testing.T) {
	mux := newTestMux(t)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET / 请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET / 应返回 404（Dashboard 已拆除 R-1372，CLI 唯一入口 R-1123），实际 %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" && strings.HasPrefix(ct, "text/html") {
		t.Errorf("GET / 不得再返回 HTML 页面（Dashboard 已拆除 R-1372），Content-Type=%s", ct)
	}
}

// TestDashboard_NoApproveCallbackPath — 源级契约（R-1372 机检等价）:
//
//	Dashboard 的 approveAction 回传（原页面文件内嵌 JS + 注册函数）不得在
//	代码中残留；approval 唯一通道=CLI（R-1123）。
func TestDashboard_NoApproveCallbackPath(t *testing.T) {
	// 1. main.go 不得再引用注册函数（"/" 注册点即 Dashboard 入口，Dashboard 已拆除 R-1372）
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("读取 cmd/goalos/main.go 失败: %v", err)
	}
	mainSrc := string(src)
	if strings.Contains(mainSrc, "Handle"+"Dash"+"board") {
		t.Error("main.go 仍引用注册函数——Dashboard 拆除不彻底（R-1372）")
	}
	if strings.Contains(mainSrc, `mux.HandleFunc("/",`) {
		t.Error("main.go 仍注册 \"/\" 路由——Dashboard 根路径入口未拆除（R-1372）")
	}

	// 2. internal/daemon 不得再存在原页面文件（approveAction 回传载体）
	if _, err := os.Stat("../internal/daemon/" + "dash" + "board.go"); err == nil {
		t.Error("internal/daemon 下原页面 Go 文件仍存在——Dashboard 拆除不彻底（R-1372）")
	}
	if _, err := os.Stat("../internal/daemon/" + "dash" + "board.html"); err == nil {
		t.Error("internal/daemon 下原页面 HTML 文件仍存在——approveAction 弹窗回传未拆除（R-1372）")
	}

	// 3. 行为级: Dashboard 曾暴露的页面路径（Dashboard 已拆除 R-1372）均 404
	mux := newTestMux(t)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	for _, path := range []string{"/", "/index.html"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s 请求失败: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s 应返回 404（Dashboard 已拆除 R-1372），实际 %d", path, resp.StatusCode)
		}
	}
}

// TestBuildHTTPMux_BaselineRoutes — 抽取 buildHTTPMux 后基线回归:
//
//	既有 API 路由不得被拆除动作波及。
func TestBuildHTTPMux_BaselineRoutes(t *testing.T) {
	mux := newTestMux(t)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	for _, tc := range []struct {
		path string
		want int
	}{
		{"/api/health", http.StatusOK},
		{"/api/approvals", http.StatusOK},
	} {
		resp, err := http.Get(srv.URL + tc.path)
		if err != nil {
			t.Fatalf("GET %s 请求失败: %v", tc.path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != tc.want {
			t.Errorf("GET %s 应返回 %d（基线路由回归 R-1372），实际 %d", tc.path, tc.want, resp.StatusCode)
		}
	}
}
