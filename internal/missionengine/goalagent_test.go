package missionengine_test

import (
	"testing"

	"github.com/goalos/goalos/internal/missionengine"
)

// mockLLM 是 LLM 客户端的 mock 实现。
type mockLLM struct {
	response string
	err      error
}

func (m *mockLLM) Chat(system, user string) (string, error) {
	return m.response, m.err
}

func TestGoalAgent_ParseValidJSON(t *testing.T) {
	agent := missionengine.NewGoalAgent(&mockLLM{
		response: `{"nodes":[{"id":"1","type":"mission","description":"分析需求"},{"id":"2","type":"mission","description":"设计方案"}],"edges":[{"from":"1","to":"2","type":"sequential"}]}`,
	})

	graph, err := agent.Plan("test", missionengine.Context{GoalID: "goal_001"})
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if len(graph.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(graph.Nodes))
	}
	if graph.Nodes[0].Description != "分析需求" {
		t.Errorf("expected 分析需求, got %s", graph.Nodes[0].Description)
	}
}

func TestGoalAgent_FallbackOnParseError(t *testing.T) {
	agent := missionengine.NewGoalAgent(&mockLLM{
		response: "这是一段文字，不是JSON",
	})

	graph, err := agent.Plan("test", missionengine.Context{GoalID: "goal_001"})
	if err != nil {
		t.Fatalf("Plan should use fallback, not fail: %v", err)
	}
	if len(graph.Nodes) != 3 {
		t.Errorf("fallback: expected 3 nodes, got %d", len(graph.Nodes))
	}
}

func TestGoalAgent_JSONWithExtraText(t *testing.T) {
	agent := missionengine.NewGoalAgent(&mockLLM{
		response: "好的，以下是任务拆解：\n```json\n{\"nodes\":[{\"id\":\"1\",\"type\":\"mission\",\"description\":\"分析需求\"}],\"edges\":[]}\n```\n希望这对你有帮助。",
	})

	graph, err := agent.Plan("test", missionengine.Context{GoalID: "goal_001"})
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if len(graph.Nodes) != 1 {
		t.Errorf("expected 1 node from embedded JSON, got %d", len(graph.Nodes))
	}
}
