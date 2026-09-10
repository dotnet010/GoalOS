//go:build linux

// runtime_wiring_linux.go——Linux 平台 Provider 注册（任务 5.2——命名空间边界族；
// 信创构建同路径——GOOS=linux 含 xinchuang 标签）。
package main

import (
	"context"
	"log"

	"github.com/goalos/goalos/internal/llm"
	goalosruntime "github.com/goalos/goalos/internal/runtime"
)

// registerPlatformProvider linux=双引擎受限档 Provider（R-1664——模式 A=bwrap 命名空间
// 族/模式 B 免 userns（landlock+seccomp）Prepare 期实证收敛；双不可用=诚实不注册——
// 注册表保持空=骨架纪律 R-1468 诚实状态）。
// 2026-09-07 修正：此前误注册裸 agentbox（双引擎从未接生产——Ubuntu 24.04 AppArmor
// 限 userns 场景=本可模式 B 承接却空注册）。
// 2026-09-08 换引擎：模式 A 位 agentbox→bwrap（Jobs 裁决——非核心免自研；agentbox
// 族全量移除）。
func registerPlatformProvider(rb *runtimeBoundary, home string, fd3dial *llm.ZoneDialer) {
	var opts []goalosruntime.LinuxOption
	if fd3dial != nil {
		// FD3 broker 拨号面=zone dialer 同源（R-1650 v2(4)——模式 B 消费）
		opts = append(opts, goalosruntime.WithLinuxDialFunc(fd3dial.DialContext))
	}
	p := goalosruntime.NewLinuxRestrictedProvider(home+"/Goals", "/tmp/goalos", opts...)
	if err := p.Prepare(context.Background(), goalosruntime.RuntimePlan{PlanID: "daemon-boot", Tier: goalosruntime.TierRestricted}); err != nil {
		log.Printf(`{"level":"WARN","msg":"Step 7c: linux 受限档 Provider Prepare 失败（诚实不注册）: %v"}`, err)
		return
	}
	if err := rb.registry.RegisterChecked(p); err != nil {
		log.Printf(`{"level":"WARN","msg":"Step 7c: linux 受限档 Provider 注册被拒: %v"}`, err)
		return
	}
	log.Printf(`{"level":"INFO","msg":"Step 7c: linux-restricted Provider 已注册（受限档 T1——双引擎：namespace/landlock+seccomp）"}`)
}
