// runtime_wiring.go——Runtime 边界组合根（任务 5.5 前置——R-1640② 会议 #255 裁决）。
// 调研决议（会议 #257——PM 调研协议）：daemon 接线开源 top3=uber/fx（运行时反射容器，
// 配置错误=启动 panic——与 fail-closed 纪律冲突）/Google wire（编译期 DI 代码生成——
// 2025 年中官方归档停维护，引入=背债）/手工组合根（Go 惯用——编译期安全零运行时开销）；
// containerd registry.Register 模式（Type/ID/Requires/InitFn）与既有 RegisterChecked 同构
// ——已在正确方向。结论=手工组合根+接缝点显式化（R-1638 决策矩阵：根本问题=接缝点存在性
// 非依赖图复杂度；不引进框架）。
package main

import (
	"context"
	"log"
	goruntime "runtime"

	"github.com/goalos/goalos/internal/config"
	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/governance"
	goalosruntime "github.com/goalos/goalos/internal/runtime"
	"github.com/goalos/goalos/pkg/events"
)

// runtimeBoundary Runtime 边界装配（注册表+解析器+验证层——执行门激活=D-1 决策项落地后）。
type runtimeBoundary struct {
	registry *goalosruntime.ProviderRegistry
	resolver *goalosruntime.Resolver
	verifier *goalosruntime.ContractVerifier
}

// runtimeWiring 组合根：平台 Provider 注册+解析器（平台探测+名单+事件发射）+
// 验证层（keyring 签发材料+吊销桥+拒绝留痕）+启动自检（边界可解析证据）。
func runtimeWiring(bus *eventbus.EventBus, home string, cfg *config.Config, gov *governance.Engine, secretKey []byte) *runtimeBoundary {
	rb := &runtimeBoundary{registry: goalosruntime.NewProviderRegistry()}

	// ①平台 Provider 注册（darwin=Seatbelt 受限档——任务 5.3；linux/windows=任务 5.1/5.2
	// 收敛前注册表保持空——骨架纪律 R-1468 诚实状态）
	if goalosruntime.DetectPlatformIsolation() >= goalosruntime.I2 && goruntime.GOOS == "darwin" {
		p := goalosruntime.NewDarwinSeatbeltProvider(home+"/Goals", "/tmp/goalos")
		if err := p.Prepare(context.Background(), goalosruntime.RuntimePlan{PlanID: "daemon-boot", Tier: goalosruntime.TierRestricted}); err != nil {
			log.Printf(`{"level":"WARN","msg":"Step 7c: darwin Provider Prepare 失败（诚实不注册）: %v"}`, err)
		} else if err := rb.registry.RegisterChecked(p); err != nil {
			log.Printf(`{"level":"WARN","msg":"Step 7c: darwin Provider 注册被拒: %v"}`, err)
		} else {
			log.Printf(`{"level":"INFO","msg":"Step 7c: darwin-seatbelt Provider 已注册（受限档 T1）"}`)
		}
	}

	// ②解析器（平台探测+名单+RuntimeSelected/Rejected 事件发射——07 §4.14）
	trusted := make([]goalosruntime.TrustedWorkload, 0, len(cfg.Daemon.TrustedWorkloads))
	for _, w := range cfg.Daemon.TrustedWorkloads {
		trusted = append(trusted, goalosruntime.TrustedWorkload{
			PublisherKey: w.PublisherKey, ArtifactHash: w.ArtifactHash, ExpiresAt: w.ExpiresAt,
		})
	}
	rb.resolver = goalosruntime.NewResolver(trusted).
		WithPlatformMaxIsolation(goalosruntime.DetectPlatformIsolation).
		WithEventHook(func(eventType string, sel goalosruntime.Selection, rejectDetail string) {
			payload := map[string]interface{}{"tier": sel.Tier.String(), "selection_reason": sel.Reason}
			if sel.MatchedWorkload != "" {
				payload["matched_workload"] = sel.MatchedWorkload // R-1589 身份标签读取点
			}
			if sel.NeedsGovernanceEscalation {
				payload["needs_governance_escalation"] = true
				payload["platform_gap"] = sel.PlatformGap
			}
			if rejectDetail != "" {
				payload["reject_detail"] = rejectDetail
			}
			bus.Publish(events.Event{Type: eventType, Source: "runtime-resolver", Payload: payload})
		})

	// ③验证层（吊销桥=IsActionRevoked 生产撤销表——R-1640②；拒绝留痕=ContractRejected）
	rb.verifier = goalosruntime.NewContractVerifier(secretKey, goalosruntime.NewNonceRegistry(),
		goalosruntime.WithRevocationChecker(func(contractID string) bool {
			// contractID=goalID/actionID → actionID 前缀形态（现行撤销表约定）
			for i := len(contractID) - 1; i >= 0; i-- {
				if contractID[i] == '/' {
					return gov.IsActionRevoked(contractID[i+1:])
				}
			}
			return gov.IsActionRevoked(contractID)
		}),
		goalosruntime.WithRejectHook(func(reason string) {
			bus.Publish(events.Event{Type: events.TypeContractRejected, Source: "runtime-verifier",
				Payload: map[string]interface{}{"reject_reason": reason}})
		}),
	)

	// ④启动自检（边界可解析证据——参考输入=典型受限档情形；结果入日志非事件流）
	sel, err := rb.resolver.Resolve(goalosruntime.ResolveInput{
		RequiresRealEnforcement: true, MinIsolation: goalosruntime.I2,
	})
	if err != nil {
		log.Printf(`{"level":"WARN","msg":"Step 7c: Runtime 边界自检未解析: %v"}`, err)
	} else {
		log.Printf(`{"level":"INFO","msg":"Step 7c: Runtime 边界自检通过——档位=%s 依据=%s 平台达成=%s"}`,
			sel.Tier, sel.Reason, goalosruntime.DetectPlatformIsolation())
	}
	return rb
}
