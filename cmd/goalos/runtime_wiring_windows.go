//go:build windows

// runtime_wiring_windows.go——Windows 平台 Provider 注册（任务 5.1——Job Object 边界族）。
package main

import (
	"context"
	"log"

	goalosruntime "github.com/goalos/goalos/internal/runtime"
)

// registerPlatformProvider windows=Job Object 受限档 Provider（Prepare 失败=诚实不注册）。
func registerPlatformProvider(rb *runtimeBoundary, home string) {
	p := goalosruntime.NewAgentboxProvider(home+"\\Goals", "\\tmp\\goalos", "windows")
	if err := p.Prepare(context.Background(), goalosruntime.RuntimePlan{PlanID: "daemon-boot", Tier: goalosruntime.TierRestricted}); err != nil {
		log.Printf(`{"level":"WARN","msg":"Step 7c: windows agentbox Provider Prepare 失败（诚实不注册）: %v"}`, err)
		return
	}
	if err := rb.registry.RegisterChecked(p); err != nil {
		log.Printf(`{"level":"WARN","msg":"Step 7c: windows agentbox Provider 注册被拒: %v"}`, err)
		return
	}
	log.Printf(`{"level":"INFO","msg":"Step 7c: windows-agentbox Provider 已注册（受限档 T1——Restricted Token+Job+Low IL+ACLs）"}`)
}
