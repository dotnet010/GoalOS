package missionengine_test

import (
	"context"
	"testing"

	"github.com/goalos/goalos/internal/llm"
	"github.com/goalos/goalos/internal/missionengine"
)

// mockLLM 是 LLMClient 接口的 mock 实现。
type mockLLM struct {
	response string
	err      error
}

func (m *mockLLM) Chat(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &llm.ChatResponse{Content: m.response}, nil
}

func (m *mockLLM) ChatStream(_ context.Context, _ *llm.ChatRequest) (<-chan llm.ChatStreamEvent, error) {
	ch := make(chan llm.ChatStreamEvent, 1)
	ch <- llm.ChatStreamEvent{Content: m.response, Done: true}
	close(ch)
	return ch, nil
}

func TestGoalAgent_ParseValidJSON(t *testing.T) {
	agent := missionengine.NewGoalAgent(&mockLLM{
		response: `{"nodes":[{"id":"1","type":"mission","description":"分析需求","action_type":"web.search","target":"需求分析"},{"id":"2","type":"mission","description":"设计方案","action_type":"shell.execute","target":"echo design"}],"edges":[{"from":"1","to":"2","type":"sequential"}]}`,
	})

	graph, err := agent.Plan(nil, nil, "", missionengine.Context{GoalID: "goal_001", GoalText: "test"})
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
	// R-724: fallbackPlan 已删除——LLM 返回非 JSON 时应诚实失败
	agent := missionengine.NewGoalAgent(&mockLLM{
		response: "这是一段文字，不是JSON",
	})

	_, err := agent.Plan(nil, nil, "", missionengine.Context{GoalID: "goal_001", GoalText: "test"})
	if err == nil {
		t.Fatal("R-724: expected error when LLM returns non-JSON, got nil")
	}
}

func TestGoalAgent_JSONWithExtraText(t *testing.T) {
	// LLM 响应包含 markdown 代码块包装——parseResponseFallback 应能提取 JSON
	agent := missionengine.NewGoalAgent(&mockLLM{
		response: "好的，以下是任务拆解：\n```json\n{\"nodes\":[{\"id\":\"1\",\"type\":\"mission\",\"description\":\"分析需求\",\"action_type\":\"web.search\",\"target\":\"test\"}],\"edges\":[]}\n```\n希望这对你有帮助。",
	})

	graph, err := agent.Plan(nil, nil, "", missionengine.Context{GoalID: "goal_001", GoalText: "test"})
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if len(graph.Nodes) != 1 {
		t.Errorf("expected 1 node from embedded JSON, got %d", len(graph.Nodes))
	}
}

func TestGoalAgent_LLMError(t *testing.T) {
	agent := missionengine.NewGoalAgent(&mockLLM{
		err: context.DeadlineExceeded,
	})

	_, err := agent.Plan(nil, nil, "", missionengine.Context{GoalID: "goal_001", GoalText: "test"})
	if err == nil {
		t.Fatal("expected error when LLM fails, got nil")
	}
}
