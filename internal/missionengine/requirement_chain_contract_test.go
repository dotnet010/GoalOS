// requirement_chain_contract_test.go——TestRequirementAdded_ConsumerChain（12 清单 G 节——
// 任务 5.8；规格=R-1597/R-1613+07 §4 R-1362）。标注=实现同步补强（非先红——诚实标注）。
// 断言：消费链端到端留痕——RequirementAdded→版本链修订→CompletionContractRecorded 落账
// （version/supersedes 链连续）→重规划触发（PlanRequested reason=requirement_added）。
package missionengine_test

import (
	"testing"
	"time"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/missionengine"
	"github.com/goalos/goalos/pkg/events"
)

// TestRequirementAdded_ConsumerChain 消费链端到端（S-247-01——全库无生产消费者缺口闭合）：
// ①首版创建（无契约时需求注入=契约创建入口）；②修订链（version 递增+supersedes 连续）；
// ③CompletionContractRecorded 落账（载荷校验过——R-770）；④重规划触发留痕。
func TestRequirementAdded_ConsumerChain(t *testing.T) {
	bus := eventbus.New()
	eng := missionengine.New(bus, nil)
	eng.Start()

	recorded := make(chan map[string]interface{}, 4)
	replan := make(chan map[string]interface{}, 4)
	bus.Subscribe(events.TypeCompletionContractRecorded, func(evt events.Event) error {
		recorded <- evt.Payload
		return nil
	})
	bus.Subscribe(events.TypePlanRequested, func(evt events.Event) error {
		replan <- evt.Payload
		return nil
	})

	// ①首版：首次需求注入
	bus.Publish(events.Event{
		Type: events.TypeRequirementAdded, GoalID: "goal-c1", Source: "test",
		Payload: map[string]interface{}{"requirement_text": "产出物须含性能基线"},
	})
	select {
	case p := <-recorded:
		if p["version"] != 1 || p["status"] != "active" {
			t.Fatalf("①首版应=version 1/active，实际: %v", p)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("①CompletionContractRecorded 未落账（消费链断裂）")
	}
	select {
	case p := <-replan:
		if p["reason"] != "requirement_added" {
			t.Fatalf("④重规划 reason 应=requirement_added（R-1400 透传），实际: %v", p["reason"])
		}
	case <-time.After(5 * time.Second):
		t.Fatal("④重规划未触发（PlanRequested 未发布）")
	}

	// ②③修订链：第二次注入→version 2+revised+supersedes 连续
	bus.Publish(events.Event{
		Type: events.TypeRequirementAdded, GoalID: "goal-c1", Source: "test",
		Payload: map[string]interface{}{"requirement_text": "交付前须人工复核"},
	})
	select {
	case p := <-recorded:
		if p["version"] != 2 || p["status"] != "revised" {
			t.Fatalf("②修订版应=version 2/revised，实际: %v", p)
		}
		if p["supersedes"] == "" || p["supersedes"] == nil {
			t.Fatal("③supersedes 必须携带旧版本契约 ID（线性链连续）")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("②修订版未落账")
	}

	// 链长断言（领域对象侧——R-1613 存在论：链管理器=领域真相）
	if got := eng.ContractChain().ChainLength("goal-c1"); got != 2 {
		t.Fatalf("版本链长应=2，实际 %d", got)
	}
}
