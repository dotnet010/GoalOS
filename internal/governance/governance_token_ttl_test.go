// Governance 行动令牌 TTL 契约测试（R-1059, 会议 #191 修正）。
// 缺陷根因（D-2/D-5）：令牌 TTL 无独立配置——auto 路径 ttl = 载荷 timeout_seconds×2
// （执行超时 30s → 偶然 60s），用户批准路径硬编码 60s。两者均不受 token_ttl 配置控制。
// 契约：TTL 唯一来源 = 引擎字段 tokenTTL（默认 300s），三条签发路径（auto / 用户批准 /
// 热重载后新签发）全部读引擎值。
package governance_test

import (
	"testing"
	"time"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/governance"
	"github.com/goalos/goalos/pkg/events"
)

const testSecretKey = "0123456789abcdef0123456789abcdef" // 32 字节固定测试密钥

// TestGovernance_TokenTTL_AutoPathUsesEngineValue
// auto 放行路径（L1）签发的令牌 TTL 必须等于 SetTokenTTL(7s) 的引擎值，
// 而非载荷 timeout_seconds×2 的偶然耦合。
func TestGovernance_TokenTTL_AutoPathUsesEngineValue(t *testing.T) {
	bus := eventbus.New()
	eng := governance.New(bus, []byte(testSecretKey))
	eng.Start()
	eng.SetTokenTTL(7 * time.Second)

	approved := make(chan events.Event, 1)
	bus.Subscribe(events.TypeActionApproved, func(evt events.Event) error {
		approved <- evt
		return nil
	})

	bus.Publish(events.Event{
		Type:   events.TypeActionScheduled,
		GoalID: "goal_ttl_auto",
		Source: "scheduler",
		Payload: map[string]interface{}{
			"action_id":       "act_ttl_001",
			"action_type":     "test.autopath", // 不在 capabilityRiskMap → L1 → 自动放行
			"timeout_seconds": float64(30),      // 执行超时：不得再影响令牌 TTL
		},
	})

	evt := <-approved
	tokenStr, _ := evt.Payload["token"].(string)
	if tokenStr == "" {
		t.Fatal("auto 路径必须签发令牌（TokenStr 空）")
	}
	claims, err := governance.VerifyToken(tokenStr, []byte(testSecretKey))
	if err != nil {
		t.Fatalf("令牌验签失败: %v", err)
	}
	if ttl := claims.ExpiresAt - claims.IssuedAt; ttl != 7 {
		t.Fatalf("令牌 TTL 必须等于引擎值 7s，got %d（禁止 2×执行超时=60s 的偶然耦合）", ttl)
	}
}

// TestGovernance_TokenTTL_UserApprovedPathUsesEngineValue
// 用户批准路径签发的令牌 TTL 必须等于 SetTokenTTL(11s) 的引擎值（此前硬编码 60s）。
func TestGovernance_TokenTTL_UserApprovedPathUsesEngineValue(t *testing.T) {
	bus := eventbus.New()
	eng := governance.New(bus, []byte(testSecretKey))
	eng.Start()
	eng.SetTokenTTL(11 * time.Second)

	approved := make(chan events.Event, 1)
	bus.Subscribe(events.TypeActionApproved, func(evt events.Event) error {
		approved <- evt
		return nil
	})

	bus.Publish(events.Event{
		Type:   events.TypeActionScheduled,
		GoalID: "goal_ttl_user",
		Source: "scheduler",
		Payload: map[string]interface{}{
			"action_id":       "act_ttl_002",
			"action_type":     "shell.execute", // L3 → 需审批
			"timeout_seconds": float64(30),
		},
	})

	// 用户批准
	bus.Publish(events.Event{
		Type:   events.TypeUserApprovedAction,
		GoalID: "goal_ttl_user",
		Source: "telegram",
		Payload: map[string]interface{}{
			"action_id": "act_ttl_002",
		},
	})

	evt := <-approved
	tokenStr, _ := evt.Payload["token"].(string)
	if tokenStr == "" {
		t.Fatal("用户批准路径必须签发令牌（TokenStr 空）")
	}
	claims, err := governance.VerifyToken(tokenStr, []byte(testSecretKey))
	if err != nil {
		t.Fatalf("令牌验签失败: %v", err)
	}
	if ttl := claims.ExpiresAt - claims.IssuedAt; ttl != 11 {
		t.Fatalf("令牌 TTL 必须等于引擎值 11s，got %d（禁止硬编码 60s）", ttl)
	}
}

// TestGovernance_TokenTTL_ConfigReloadedAffectsNewIssuance
// R-1058+R-1059: ConfigReloaded 事件携带 token_ttl_seconds 更新引擎值；
// 之后新签发的令牌使用新 TTL（nginx 代际模型——只影响新一代签发）。
func TestGovernance_TokenTTL_ConfigReloadedAffectsNewIssuance(t *testing.T) {
	bus := eventbus.New()
	eng := governance.New(bus, []byte(testSecretKey))
	eng.Start()
	eng.SetTokenTTL(300 * time.Second)

	bus.Publish(events.Event{
		Type:   events.TypeConfigReloaded,
		Source: "daemon",
		Payload: map[string]interface{}{
			"token_ttl_seconds":        float64(12),
			"approval_timeout_seconds": float64(300),
			"autonomy_level":           "approve",
		},
	})

	approved := make(chan events.Event, 1)
	bus.Subscribe(events.TypeActionApproved, func(evt events.Event) error {
		approved <- evt
		return nil
	})

	bus.Publish(events.Event{
		Type:   events.TypeActionScheduled,
		GoalID: "goal_ttl_reload",
		Source: "scheduler",
		Payload: map[string]interface{}{
			"action_id":   "act_ttl_003",
			"action_type": "test.autopath",
		},
	})

	evt := <-approved
	tokenStr, _ := evt.Payload["token"].(string)
	if tokenStr == "" {
		t.Fatal("reload 后新签发的令牌不得为空")
	}
	claims, err := governance.VerifyToken(tokenStr, []byte(testSecretKey))
	if err != nil {
		t.Fatalf("令牌验签失败: %v", err)
	}
	if ttl := claims.ExpiresAt - claims.IssuedAt; ttl != 12 {
		t.Fatalf("热重载后新签发令牌 TTL 必须为 12s，got %d", ttl)
	}
}
