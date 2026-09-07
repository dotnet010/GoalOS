//go:build darwin

// runtime_wiring_darwin.go——darwin 平台 Provider 注册（任务 5.3 收敛产物入生产组合根）。
package main

import (
	"context"
	"log"

	"github.com/goalos/goalos/internal/llm"
	goalosruntime "github.com/goalos/goalos/internal/runtime"
)

// registerPlatformProvider darwin=Seatbelt 受限档 Provider（Prepare 失败=诚实不注册）。
func registerPlatformProvider(rb *runtimeBoundary, home string, fd3dial *llm.ZoneDialer) {
	if goalosruntime.DetectPlatformIsolation() < goalosruntime.I2 {
		return
	}
	var opts []goalosruntime.DarwinOption
	if fd3dial != nil {
		// FD3 broker 拨号面=zone dialer 同源（R-1650 v2④——darwin 面同构注入）
		opts = append(opts, goalosruntime.WithDarwinDialFunc(fd3dial.DialContext))
	}
	p := goalosruntime.NewDarwinSeatbeltProvider(home+"/Goals", "/tmp/goalos", opts...)
	if err := p.Prepare(context.Background(), goalosruntime.RuntimePlan{PlanID: "daemon-boot", Tier: goalosruntime.TierRestricted}); err != nil {
		log.Printf(`{"level":"WARN","msg":"Step 7c: darwin Provider Prepare 失败（诚实不注册）: %v"}`, err)
		return
	}
	if err := rb.registry.RegisterChecked(p); err != nil {
		log.Printf(`{"level":"WARN","msg":"Step 7c: darwin Provider 注册被拒: %v"}`, err)
		return
	}
	log.Printf(`{"level":"INFO","msg":"Step 7c: darwin-seatbelt Provider 已注册（受限档 T1）"}`)
}
