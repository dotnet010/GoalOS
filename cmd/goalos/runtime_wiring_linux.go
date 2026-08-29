//go:build linux

// runtime_wiring_linux.go——Linux 平台 Provider 注册（任务 5.2——命名空间边界族；
// 信创构建同路径——GOOS=linux 含 xinchuang 标签）。
package main

import (
	"context"
	"log"

	goalosruntime "github.com/goalos/goalos/internal/runtime"
)

// registerPlatformProvider linux=命名空间受限档 Provider（Prepare 失败=诚实不注册——
// 缺 CAP_SYS_ADMIN/内核限制场景如实降级：注册表保持空=骨架纪律 R-1468 诚实状态）。
func registerPlatformProvider(rb *runtimeBoundary, home string) {
	p := goalosruntime.NewAgentboxProvider(home+"/Goals", "/tmp/goalos", "linux")
	if err := p.Prepare(context.Background(), goalosruntime.RuntimePlan{PlanID: "daemon-boot", Tier: goalosruntime.TierRestricted}); err != nil {
		log.Printf(`{"level":"WARN","msg":"Step 7c: linux agentbox Provider Prepare 失败（诚实不注册）: %v"}`, err)
		return
	}
	if err := rb.registry.RegisterChecked(p); err != nil {
		log.Printf(`{"level":"WARN","msg":"Step 7c: linux agentbox Provider 注册被拒: %v"}`, err)
		return
	}
	log.Printf(`{"level":"INFO","msg":"Step 7c: linux-agentbox Provider 已注册（受限档 T1——namespace+Landlock+seccomp）"}`)
}
