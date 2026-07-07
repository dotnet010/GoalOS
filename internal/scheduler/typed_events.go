// Package scheduler — v0.2.0 Week 3: Typed Event Payloads
// 5 核心事件 Go struct 替代 map[string]interface{}（B2/C16）。
package scheduler

// GoalCreatedPayload 是 GoalCreated 事件的 typed payload。
type GoalCreatedPayload struct {
	GoalID      string   `json:"goal_id"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// ActionScheduledPayload 是 ActionScheduled 事件的 typed payload。
type ActionScheduledPayload struct {
	ActionID             string   `json:"action_id"`
	ActionType           string   `json:"action_type"`
	Target               string   `json:"target"`
	Source               string   `json:"source"`
	RequiredCapabilities []string `json:"required_capabilities"`
	TimeoutSeconds       int      `json:"timeout_seconds"`
	RiskLevelPre         string   `json:"risk_level_pre"`
}

// ActionCompletedPayload 是 ActionCompleted 事件的 typed payload。
type ActionCompletedPayload struct {
	ActionID           string   `json:"action_id"`
	Status             string   `json:"status"`
	Output             string   `json:"output,omitempty"`
	ArtifactsProduced  []string `json:"artifacts_produced,omitempty"`
	DurationMs         int      `json:"duration_ms"`
	Tokens             int      `json:"tokens,omitempty"`
}

// GoalCompletedPayload 是 GoalCompleted 事件的 typed payload。
type GoalCompletedPayloadTyped struct {
	GoalID            string `json:"goal_id"`
	ArtifactPath      string `json:"artifact_path"`
	DurationSeconds   int    `json:"duration_seconds"`
	TotalActions      int    `json:"total_actions"`
	SucceededActions  int    `json:"succeeded_actions"`
	FailedActions     int    `json:"failed_actions"`
	TotalTokens       int    `json:"total_tokens"`
	HumanInterventions int   `json:"human_interventions"`
}

// GoalFailedPayload 是 GoalFailed 事件的 typed payload。
type GoalFailedPayload struct {
	GoalID    string `json:"goal_id"`
	Reason    string `json:"reason"`
	Error     string `json:"error"`
	ErrorHint string `json:"error_hint"`
}
