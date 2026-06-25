package statestore_test

import (
	"encoding/json"
	"testing"

	"github.com/goalos/goalos/internal/statestore"
)

// TestPipelineState_RoundTrip 验证 PipelineState 序列化往返（v1.1.0）。
func TestPipelineState_RoundTrip(t *testing.T) {
	ps := &statestore.PipelineState{
		ResumePoint:     "node-3",
		ResumePrimitive: "decide",
		WaitReason:      "approval",
		TimeoutAt:       "2026-06-26T12:00:00Z",
		PendingActionIDs: []string{"act-5", "act-6"},
	}

	data, err := json.Marshal(ps)
	if err != nil {
		t.Fatalf("PipelineState marshal failed: %v", err)
	}

	var restored statestore.PipelineState
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("PipelineState unmarshal failed: %v", err)
	}

	if restored.ResumePoint != ps.ResumePoint {
		t.Errorf("ResumePoint: got %q, want %q", restored.ResumePoint, ps.ResumePoint)
	}
	if restored.ResumePrimitive != ps.ResumePrimitive {
		t.Errorf("ResumePrimitive: got %q, want %q", restored.ResumePrimitive, ps.ResumePrimitive)
	}
	if restored.WaitReason != ps.WaitReason {
		t.Errorf("WaitReason: got %q, want %q", restored.WaitReason, ps.WaitReason)
	}
	if restored.TimeoutAt != ps.TimeoutAt {
		t.Errorf("TimeoutAt: got %q, want %q", restored.TimeoutAt, ps.TimeoutAt)
	}
	if len(restored.PendingActionIDs) != 2 {
		t.Errorf("PendingActionIDs: got %d items, want 2", len(restored.PendingActionIDs))
	}
}

// TestGoalState_WithPipelineState 验证 PipelineState 嵌入 GoalState 的序列化。
func TestGoalState_WithPipelineState(t *testing.T) {
	gs := &statestore.GoalState{
		GoalID:        "goal-001",
		InternalState: "running",
		PipelineState: &statestore.PipelineState{
			ResumePoint:     "node-2",
			ResumePrimitive: "decide",
			WaitReason:      "dependency",
			TimeoutAt:       "2026-06-26T12:00:00Z",
		},
	}

	data, err := json.Marshal(gs)
	if err != nil {
		t.Fatalf("GoalState marshal failed: %v", err)
	}

	var restored statestore.GoalState
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("GoalState unmarshal failed: %v", err)
	}

	if restored.PipelineState == nil {
		t.Fatal("PipelineState is nil after round-trip")
	}
	if restored.PipelineState.ResumePrimitive != "decide" {
		t.Errorf("ResumePrimitive: got %q, want %q", restored.PipelineState.ResumePrimitive, "decide")
	}
}

// TestGoalState_WithoutPipelineState 验证无 Wait 状态时 PipelineState 为 nil。
func TestGoalState_WithoutPipelineState(t *testing.T) {
	gs := &statestore.GoalState{
		GoalID:        "goal-002",
		InternalState: "running",
	}

	data, err := json.Marshal(gs)
	if err != nil {
		t.Fatalf("GoalState marshal failed: %v", err)
	}

	var restored statestore.GoalState
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("GoalState unmarshal failed: %v", err)
	}

	if restored.PipelineState != nil {
		t.Fatal("PipelineState should be nil when Goal is not in Wait state")
	}
}
