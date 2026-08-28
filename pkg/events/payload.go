// Package events — v0.2.0 Week 1: typed event payload + Validatable 接口
// M1-M8 Validate() 实现。EventBus.Publish() 自动调用（R-770）。
package events

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

// ─── Validatable 接口（H8 + R-770）─────────────────────────────

// Validatable 由所有跨模块传递的 event payload 实现。
// EventBus.Publish() 在投递事件前自动调用 Validate()。
type Validatable interface {
	Validate() error
}

// ─── M1 + M6: GoalCreatedPayload ──────────────────────────────────

// GoalCreatedPayload 是 GoalCreated 事件的 payload。
type GoalCreatedPayload struct {
	GoalID      string `json:"goal_id"`      // M1: MUST 非空
	Title       string `json:"title"`        // 用户输入
	Description string `json:"description"`  // 可选
	Tags        []string `json:"tags"`       // 可选
}

func (p GoalCreatedPayload) Validate() error {
	// M1: GoalID 非空
	if p.GoalID == "" {
		return fmt.Errorf("GoalID: nonempty")
	}
	// M1: GoalID 不能仅含空白
	for _, r := range p.GoalID {
		if r != ' ' && r != '\t' && r != '\n' {
			goto validGoalID
		}
	}
	return fmt.Errorf("GoalID: nonempty (whitespace-only)")
validGoalID:

	// M6: goal 非空
	if p.Title == "" {
		return fmt.Errorf("title: nonempty")
	}
	// M6: len < 10000
	if len(p.Title) > 10000 {
		return fmt.Errorf("title: len<10000 (actual=%d)", len(p.Title))
	}
	// M6: UTF-8
	if !utf8.ValidString(p.Title) {
		return fmt.Errorf("title: utf8")
	}
	// M6: 无 HTML 标签
	for i := 0; i < len(p.Title)-1; i++ {
		if p.Title[i] == '<' {
			for j := i + 1; j < len(p.Title); j++ {
				if p.Title[j] == '>' {
					return fmt.Errorf("title: no_html (found <%s>)", p.Title[i+1:j])
				}
			}
		}
	}
	return nil
}

// ─── M2: CompletionCriteria ────────────────────────────────────────

// CompletionCriteria 是 Agent.Align() 的输出。
type CompletionCriteria struct {
	GoalType          string `json:"goal_type"`
	SuccessDefinition string `json:"success_definition"`
}

func (p CompletionCriteria) Validate() error {
	// M2: goal_type 非空
	if p.GoalType == "" {
		return fmt.Errorf("GoalType: nonempty")
	}
	// M2: SuccessDefinition 非空
	if p.SuccessDefinition == "" {
		return fmt.Errorf("SuccessDefinition: nonempty")
	}
	// M2: goal_type 合法值检查
	validTypes := map[string]bool{
		"code_generation": true, "data_analysis": true, "research": true,
		"content_creation": true, "automation": true, "generic": true,
	}
	if !validTypes[p.GoalType] {
		return fmt.Errorf("GoalType: invalid value %q (expected one of: code_generation, data_analysis, research, content_creation, automation, generic)", p.GoalType)
	}
	return nil
}

// ─── M3: IPCResultPayload ──────────────────────────────────────────

// IPCResultPayload 是 Plugin 子进程返回的消息 payload。
type IPCResultPayload struct {
	Type     string `json:"type"`      // "result" | "error"
	ActionID string `json:"action_id"`
	Status   string `json:"status"`    // "success" | "failure"
	Output   string `json:"output"`
}

func (p IPCResultPayload) Validate() error {
	// M3: type 枚举检查
	if p.Type != "result" && p.Type != "error" {
		return fmt.Errorf("type: invalid value %q (expected result|error)", p.Type)
	}
	// M3: status 枚举检查
	if p.Status != "success" && p.Status != "failure" {
		return fmt.Errorf("status: invalid value %q (expected success|failure)", p.Status)
	}
	// M3: action_id 非空
	if p.ActionID == "" {
		return fmt.Errorf("actionID: nonempty")
	}
	// M3: output ≤ 64KB
	if len(p.Output) > 64*1024 {
		return fmt.Errorf("output: len<=64KB (actual=%d)", len(p.Output))
	}
	return nil
}

// ─── M4 + M8: GoalCompletedPayload ─────────────────────────────────

// GoalCompletedPayload 是 GoalCompleted 事件的 payload。
type GoalCompletedPayload struct {
	GoalID       string `json:"goal_id"`
	ArtifactPath string `json:"artifact_path"`
	GoalState    string `json:"goal_state"` // 当前 Goal 状态——用于 M8 防重复
}

func (p GoalCompletedPayload) Validate() error {
	// M4: artifact_path 非空
	if p.ArtifactPath == "" {
		return fmt.Errorf("ArtifactPath: nonempty")
	}
	// M8: Goal 状态不能为 Failed（防重复发布）
	if p.GoalState == "Failed" {
		return fmt.Errorf("GoalState: cannot be Failed——GoalCompleted 不可在 GoalFailed 后发布")
	}
	return nil
}

// ─── M7: FileContentPayload ────────────────────────────────────────

// FileContentPayload 是 ContextEngine 读取文件时的 payload。
type FileContentPayload struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

func (p FileContentPayload) Validate() error {
	// M7: 文件大小 ≤ 10MB
	const maxSize int64 = 10 * 1024 * 1024
	if p.Size > maxSize {
		return fmt.Errorf("size: len<=%d (actual=%d)", maxSize, p.Size)
	}
	return nil
}

// PayloadToMap 将 typed payload 转换为 map[string]interface{}（R-828 统一）。
func PayloadToMap(v interface{}) map[string]interface{} {
	switch p := v.(type) {
	case ActionScheduledPayload:
		return map[string]interface{}{
			"action_id": p.ActionID, "action_type": p.ActionType, "source": p.Source,
			"timeout_seconds": p.TimeoutSeconds, "risk_level_pre": p.RiskLevelPre,
			"required_capabilities": p.RequiredCapabilities, "target": p.Target,
		}
	case ActionCompletedPayload:
		return map[string]interface{}{
			"action_id": p.ActionID, "status": p.Status, "output": p.Output,
			"artifacts_produced": p.ArtifactsProduced, "duration_ms": p.DurationMs, "tokens": p.Tokens,
		}
	case GoalCompletedPayloadV2:
		return map[string]interface{}{
			"goal_id": p.GoalID, "artifact_path": p.ArtifactPath, "goal_state": p.GoalState,
			"duration_seconds": p.DurationSeconds, "total_actions": p.TotalActions,
			"succeeded_actions": p.SucceededActions, "failed_actions": p.FailedActions,
			"total_tokens": p.TotalTokens, "human_interventions": p.HumanInterventions,
		}
	case GoalFailedPayload:
		return map[string]interface{}{
			"goal_id": p.GoalID, "reason": p.Reason,
			"error": p.Error, "error_hint": p.ErrorHint,
		}
	default:
		data, _ := json.Marshal(v)
		var result map[string]interface{}
		json.Unmarshal(data, &result)
		return result
	}
}

// ─── R-828 Step 1: 从 internal/scheduler 迁移核心 payload ──────────

// ActionScheduledPayload 是 ActionScheduled 事件的 typed payload。
type ActionScheduledPayload struct {
	ActionID             string   `json:"action_id"`
	ActionType           string   `json:"action_type,omitempty"`
	Target               string   `json:"target,omitempty"`
	Source               string   `json:"source,omitempty"`
	RequiredCapabilities []string `json:"required_capabilities,omitempty"`
	TimeoutSeconds       int      `json:"timeout_seconds"`
	RiskLevelPre         string   `json:"risk_level_pre,omitempty"`
}

func (p ActionScheduledPayload) EventType() string { return "ActionScheduled" }
func (p ActionScheduledPayload) Validate() error {
	if p.ActionID == "" {
		return fmt.Errorf("ActionScheduledPayload: ActionID is required")
	}
	return nil
}

// ActionCompletedPayload 是 ActionCompleted 事件的 typed payload。
type ActionCompletedPayload struct {
	ActionID          string   `json:"action_id"`
	Status            string   `json:"status"`
	Output            string   `json:"output,omitempty"`
	ArtifactsProduced []string `json:"artifacts_produced,omitempty"`
	DurationMs        int      `json:"duration_ms"`
	Tokens            int      `json:"tokens"`
}

func (p ActionCompletedPayload) EventType() string { return "ActionCompleted" }
func (p ActionCompletedPayload) Validate() error {
	if p.ActionID == "" {
		return fmt.Errorf("ActionCompletedPayload: ActionID is required")
	}
	if p.Status != "success" && p.Status != "failure" {
		return fmt.Errorf("ActionCompletedPayload: Status must be success|failure, got %q", p.Status)
	}
	return nil
}

// GoalFailedPayload 是 GoalFailed 事件的 typed payload。
type GoalFailedPayload struct {
	GoalID    string `json:"goal_id"`
	Reason    string `json:"reason"`
	Error     string `json:"error,omitempty"`
	ErrorHint string `json:"error_hint,omitempty"`
}

func (p GoalFailedPayload) EventType() string { return "GoalFailed" }
func (p GoalFailedPayload) Validate() error {
	if p.GoalID == "" {
		return fmt.Errorf("GoalFailedPayload: GoalID is required")
	}
	if p.Reason == "" {
		return fmt.Errorf("GoalFailedPayload: Reason is required")
	}
	return nil
}

// GoalCompletedPayloadV2 是 GoalCompleted 事件的增强 typed payload（R-828）。
type GoalCompletedPayloadV2 struct {
	GoalID             string `json:"goal_id"`
	ArtifactPath       string `json:"artifact_path"`
	GoalState          string `json:"goal_state,omitempty"`
	DurationSeconds    int    `json:"duration_seconds"`
	TotalActions       int    `json:"total_actions"`
	SucceededActions   int    `json:"succeeded_actions"`
	FailedActions      int    `json:"failed_actions"`
	TotalTokens        int    `json:"total_tokens"`
	HumanInterventions int    `json:"human_interventions"`
}

func (p GoalCompletedPayloadV2) EventType() string { return "GoalCompleted" }
func (p GoalCompletedPayloadV2) Validate() error {
	if p.GoalID == "" {
		return fmt.Errorf("GoalCompletedPayloadV2: GoalID is required")
	}
	if p.ArtifactPath == "" {
		return fmt.Errorf("GoalCompletedPayloadV2: ArtifactPath is required")
	}
	if p.GoalState == "Failed" {
		return fmt.Errorf("GoalCompletedPayloadV2: GoalState cannot be Failed")
	}
	return nil
}

// ─── v0.3.1 Runtime 族 payload（07 §4.14——密钥/签名/敏感载荷永不入事件 R-1480）───

// ContractIssuedPayload / ContractRejectedPayload 契约族（Publisher=治理层签发/验证层拒绝）。
type ContractIssuedPayload struct {
	ContractID               string `json:"contract_id"`
	Subject                  string `json:"subject"`                  // WorkloadIdentity 名单条目引用
	ProfileDigest            string `json:"profile_digest"`           // hex(32B)
	PolicyRevision           string `json:"policy_revision"`
	RequiresRealEnforcement  bool   `json:"requires_real_enforcement"`
	MinIsolation             string `json:"min_isolation"`            // I 族值
	IssuerKeyID              string `json:"issuer_key_id"`
	ExpiresAt                string `json:"expires_at"`
}

// Validate 校验 ContractIssuedPayload（契约 ID 非空）。
func (p ContractIssuedPayload) Validate() error {
	if p.ContractID == "" {
		return fmt.Errorf("ContractIssuedPayload: ContractID is required")
	}
	return nil
}

// ContractRejectedPayload 契约拒绝（reject_reason 枚举=07 §4.14 封闭五值）。
type ContractRejectedPayload struct {
	ContractID   string `json:"contract_id"`
	Subject      string `json:"subject"`
	ProfileDigest string `json:"profile_digest"`
	PolicyRevision string `json:"policy_revision"`
	RequiresRealEnforcement bool `json:"requires_real_enforcement"`
	MinIsolation string `json:"min_isolation"`
	IssuerKeyID  string `json:"issuer_key_id"`
	ExpiresAt    string `json:"expires_at"`
	RejectReason string `json:"reject_reason"` // signature_invalid|expired|revoked|profile_digest_mismatch|nonce_replayed
}

// Validate 校验 ContractRejectedPayload（reject_reason 枚举封闭）。
func (p ContractRejectedPayload) Validate() error {
	if p.ContractID == "" {
		return fmt.Errorf("ContractRejectedPayload: ContractID is required")
	}
	switch p.RejectReason {
	case "signature_invalid", "expired", "revoked", "profile_digest_mismatch", "nonce_replayed", "invalid_fields": // R-1628 补 invalid_fields（07 §4.14 六值）
		return nil
	default:
		return fmt.Errorf("ContractRejectedPayload: reject_reason 非法值 %q", p.RejectReason)
	}
}

// RuntimeSelectedPayload / RuntimeSelectionRejectedPayload 解析族（Publisher=Runtime Resolver）。
type RuntimeSelectedPayload struct {
	ContractID      string `json:"contract_id"`
	SessionID       string `json:"session_id"`
	Tier            string `json:"tier"`             // T0|T1|T2|T3（工程值；用户呈现经产品语义词映射）
	Provider        string `json:"provider"`         // RuntimeSelected 必填
	SelectionReason string `json:"selection_reason"` // 决策表命中行标识（机器值——R-1527 映射表转换）
	MatchedWorkload string `json:"matched_workload,omitempty"` // 行 1 命中=名单条目 publisher_key 短码前 8 字符；非名单路径=省略（R-1589，MINOR 兼容）
}

// Validate 校验 RuntimeSelectedPayload。
func (p RuntimeSelectedPayload) Validate() error {
	if p.ContractID == "" || p.SessionID == "" || p.Tier == "" || p.Provider == "" {
		return fmt.Errorf("RuntimeSelectedPayload: contract_id/session_id/tier/provider 必填")
	}
	return nil
}

// RuntimeSelectionRejectedPayload 解析拒绝（reject_detail=各候选筛除原因列表）。
type RuntimeSelectionRejectedPayload struct {
	ContractID   string `json:"contract_id"`
	SessionID    string `json:"session_id"`
	Tier         string `json:"tier"`
	Provider     string `json:"provider"`
	SelectionReason string `json:"selection_reason"`
	RejectDetail string `json:"reject_detail"`
}

// Validate 校验 RuntimeSelectionRejectedPayload。
func (p RuntimeSelectionRejectedPayload) Validate() error {
	if p.ContractID == "" {
		return fmt.Errorf("RuntimeSelectionRejectedPayload: contract_id 必填")
	}
	return nil
}

// RuntimeLeasePayload 租约族四事件共用（RuntimeAcquired/RuntimeAcquireFailed/PrecheckFailed/RuntimeReleased）。
type RuntimeLeasePayload struct {
	ContractID   string `json:"contract_id"`
	SessionID    string `json:"session_id"`
	LeaseID      string `json:"lease_id"`
	Provider     string `json:"provider"`
	Tier         string `json:"tier"`
	HandleState  string `json:"handle_state"`   // 租约状态机当前值
	WarmPoolHit  bool   `json:"warm_pool_hit"`  // RuntimeAcquired 必填——热池命中率指标数据源
	ErrorCode    string `json:"error_code,omitempty"` // 失败两事件=09 RTM 族错误码
}

// Validate 校验 RuntimeLeasePayload。
func (p RuntimeLeasePayload) Validate() error {
	if p.ContractID == "" || p.SessionID == "" || p.LeaseID == "" {
		return fmt.Errorf("RuntimeLeasePayload: contract_id/session_id/lease_id 必填")
	}
	return nil
}

// SessionEscalatedPayload 升级完成（新会话挂同卷——升级=新会话挂同一工作区卷，不做句柄迁移）。
type SessionEscalatedPayload struct {
	OldSessionID      string `json:"old_session_id"`
	NewSessionID      string `json:"new_session_id"`
	OldContractID     string `json:"old_contract_id"`
	NewContractID     string `json:"new_contract_id"`
	FromTier          string `json:"from_tier"`
	ToTier            string `json:"to_tier"`
	WorkspaceVolumeID string `json:"workspace_volume_id"` // 同一卷引用不变
	EscalationSignal  string `json:"escalation_signal"`   // capability_proxy|risk_reeval（R-1560 同枚举随链传递）
}

// Validate 校验 SessionEscalatedPayload。
func (p SessionEscalatedPayload) Validate() error {
	if p.OldSessionID == "" || p.NewSessionID == "" || p.NewContractID == "" {
		return fmt.Errorf("SessionEscalatedPayload: old_session_id/new_session_id/new_contract_id 必填")
	}
	switch p.EscalationSignal {
	case "capability_proxy", "risk_reeval":
	default:
		return fmt.Errorf("SessionEscalatedPayload: escalation_signal 非法值 %q", p.EscalationSignal)
	}
	return nil
}

// ProviderDegradedPayload / ProviderStateChangedPayload Provider 族（Publisher=Provider 健康探测）。
type ProviderDegradedPayload struct {
	Provider           string `json:"provider"`
	Version            string `json:"version"`
	FromState          string `json:"from_state"`
	ToState            string `json:"to_state"`
	DegradedEvidence   string `json:"degraded_evidence"`   // ProviderDegraded 必填——降级证据描述
	CapabilitySnapshot string `json:"capability_snapshot"` // PlatformCapability 摘要引用
}

// Validate 校验 ProviderDegradedPayload。
func (p ProviderDegradedPayload) Validate() error {
	if p.Provider == "" || p.DegradedEvidence == "" {
		return fmt.Errorf("ProviderDegradedPayload: provider/degraded_evidence 必填")
	}
	return nil
}

// ProviderStateChangedPayload Provider 状态迁移。
type ProviderStateChangedPayload struct {
	Provider           string `json:"provider"`
	Version            string `json:"version"`
	FromState          string `json:"from_state"`
	ToState            string `json:"to_state"`
	DegradedEvidence   string `json:"degraded_evidence,omitempty"`
	CapabilitySnapshot string `json:"capability_snapshot,omitempty"`
}

// Validate 校验 ProviderStateChangedPayload。
func (p ProviderStateChangedPayload) Validate() error {
	if p.Provider == "" || p.ToState == "" {
		return fmt.Errorf("ProviderStateChangedPayload: provider/to_state 必填")
	}
	return nil
}

// EscalationSignaledPayload 升级信号（能力代理层检测/治理 Risk 重评——触发集合封闭五值 R-1618）。
type EscalationSignaledPayload struct {
	SessionID     string `json:"session_id"`
	ContractID    string `json:"contract_id"`
	SignalSource  string `json:"signal_source"`  // capability_proxy|risk_reeval
	SignalDetail  string `json:"signal_detail"`  // 越界能力请求描述/重评依据（=触发值，审计可回放 R-1618）
	CurrentTier   string `json:"current_tier"`
}

// Validate 校验 EscalationSignaledPayload。
func (p EscalationSignaledPayload) Validate() error {
	if p.SessionID == "" || p.ContractID == "" {
		return fmt.Errorf("EscalationSignaledPayload: session_id/contract_id 必填")
	}
	switch p.SignalSource {
	case "capability_proxy", "risk_reeval":
	default:
		return fmt.Errorf("EscalationSignaledPayload: signal_source 非法值 %q", p.SignalSource)
	}
	return nil
}
