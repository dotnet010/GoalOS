//go:build windows

// runtime_wiring_windows.go——Windows 平台 Provider 注册（R-1648——AppContainer 基座）。
package main

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"github.com/goalos/goalos/internal/llm"
	goalosruntime "github.com/goalos/goalos/internal/runtime"
)

// registerPlatformProvider windows=AppContainer 受限档 Provider（R-1648——
// 取代 Job Object 族：读写双禁闭+网络禁闭（零 capability）+win32k 子集过滤，
// 内核级绞杀=Job KILL_ON_JOB_CLOSE。Prepare 失败=诚实不注册——fail-closed）。
// toolchains=nil：具名能力授予随契约声明动态（R-1659 v3），daemon 无静态工具链表。
// tmpDir=os.TempDir() 下 goalos 专属子目录——原 "\tmp\goalos"=盘符根路径（随工作目录
// 盘符漂移且通常不存在），2026-08-30 实机复核修正。
func registerPlatformProvider(rb *runtimeBoundary, home string, fd3dial *llm.ZoneDialer) {
	var opts []goalosruntime.WinACOption
	if fd3dial != nil {
		// FD3 broker 拨号面=zone dialer 同源（R-1650 v2④——网域分类留痕不旁路）
		opts = append(opts, goalosruntime.WithDialFunc(fd3dial.DialContext))
	}
	p := goalosruntime.NewWinACProvider(filepath.Join(home, "Goals"), filepath.Join(os.TempDir(), "goalos"), nil, opts...)
	if err := p.Prepare(context.Background(), goalosruntime.RuntimePlan{PlanID: "daemon-boot", Tier: goalosruntime.TierRestricted}); err != nil {
		log.Printf(`{"level":"WARN","msg":"Step 7c: windows WinAC Provider Prepare 失败（诚实不注册）: %v"}`, err)
		return
	}
	if err := rb.registry.RegisterChecked(p); err != nil {
		log.Printf(`{"level":"WARN","msg":"Step 7c: windows WinAC Provider 注册被拒: %v"}`, err)
		return
	}
	log.Printf(`{"level":"INFO","msg":"Step 7c: windows-winac Provider 已注册（受限档 T1——AppContainer+Job KILL_ON_JOB_CLOSE）"}`)
}
