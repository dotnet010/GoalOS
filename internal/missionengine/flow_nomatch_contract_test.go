// Flow 无匹配必须生成确认 Flow 契约（R-1368 / R-1383）。
//
// 契约（resolutions.yaml R-1368）:
//   "builtin flows v0.1 约束 + FlowRecommender 静默回退删除（R-791 确认流程唯一路径）"。
//   → flow 无匹配时唯一路径 = 确认流程（FlowGenerationRequested → 用户确认），
//     禁止静默回退 generic-v1。
//
// 先红状态（阶段 3.5 测试先行闸口——用例先红）:
//   pkg/events 注册表无 FlowGenerationRequested 事件类型；missionengine 对未知
//   flow_name 直接透传给 Agent.Plan（静默继续）。
//   红锚: 构造无匹配输入（flow_name 在注册表零命中）→
//     ① 未产出确认 Flow 标记（红）
//     ② 未先确认即产出 MissionGenerated（静默回退，红）
//
// 转绿任务: 7.23（C-2 表）——FlowRecommender 确认流程落地（R-1368）后本测试转绿。
//
// 断言方式: 行为断言（事件流线格式 + 时序）——禁止读源码文本断言。
package missionengine_test

import (
	"sync"
	"testing"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/missionengine"
	"github.com/goalos/goalos/pkg/events"
)

// flowGenerationRequestedWire 是确认 Flow 生成事件的线格式（wire value）。
// pkg/events 注册表尚未注册该类型（R-1368 先红）——测试以线格式锚定契约，
// 转绿时实现必须发布该 wire 值（07 事件注册表登记）。
const flowGenerationRequestedWire = "FlowGenerationRequested"

// TestFlowNoMatch_GeneratesConfirmFlow — flow 无匹配必须生成确认 Flow。
//
// MUST 无匹配输入产出 FlowGenerationRequested（确认流程唯一路径 R-1368）
// MUST 确认请求先于 MissionGenerated（禁止静默回退 generic-v1）
// MUST 禁止无确认标记直接产出 MissionGenerated
func TestFlowNoMatch_GeneratesConfirmFlow(t *testing.T) {
	bus := eventbus.New()
	engine := missionengine.New(bus, &missionengine.StubAgent{})
	engine.Start()

	// 事件采集器（EventBus 对核心事件同步分发——Publish 返回时已投递完毕）
	var mu sync.Mutex
	var got []string
	record := func(typ string) func(events.Event) error {
		return func(evt events.Event) error {
			mu.Lock()
			got = append(got, typ)
			mu.Unlock()
			return nil
		}
	}
	bus.Subscribe(flowGenerationRequestedWire, record(flowGenerationRequestedWire))
	bus.Subscribe(events.TypeMissionGenerated, record(events.TypeMissionGenerated))
	bus.Subscribe(events.TypeUserConfirmed, record(events.TypeUserConfirmed))
	bus.Subscribe(events.TypeUserRejected, record(events.TypeUserRejected))

	// 构造无匹配输入: 空 ruleset 等价物——flow_name 在 FlowRegistry 零命中
	bus.Publish(events.Event{
		Type:   events.TypePlanRequested,
		GoalID: "goal_flow_no_match",
		Source: "scheduler",
		Payload: map[string]interface{}{
			"goal_text": "无匹配 flow 的测试目标",
			"flow_name": "no-such-flow-template-v99",
		},
	})

	mu.Lock()
	confirmIdx, genIdx := -1, -1
	for i, typ := range got {
		if typ == flowGenerationRequestedWire && confirmIdx == -1 {
			confirmIdx = i
		}
		if typ == events.TypeMissionGenerated && genIdx == -1 {
			genIdx = i
		}
	}
	hasConfirmFlow := confirmIdx != -1
	hasMissionGenerated := genIdx != -1
	mu.Unlock()

	// 契约 1: flow 无匹配必须生成确认 Flow（FlowGenerationRequested → 用户确认）——先红
	if !hasConfirmFlow {
		t.Errorf("flow 无匹配未生成确认 Flow: 事件流 %v 缺 %q——R-1368 先红（确认流程是唯一路径，禁止静默回退）", got, flowGenerationRequestedWire)
	}
	// 契约 2: 禁止静默回退——无确认请求即产出 MissionGenerated（generic 路径）——先红
	if hasMissionGenerated && !hasConfirmFlow {
		t.Errorf("flow 无匹配仍静默产出 MissionGenerated（事件流 %v）——静默回退 generic-v1 禁止（R-1368 先红）", got)
	}
	// 契约 3: 时序——确认请求必须先于执行产出（确认流程唯一路径 R-1368）
	if hasConfirmFlow && hasMissionGenerated && confirmIdx > genIdx {
		t.Errorf("确认 Flow 必须先于 MissionGenerated（确认流程唯一路径 R-1368），事件流 %v", got)
	}
}
