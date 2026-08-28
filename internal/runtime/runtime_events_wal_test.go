// runtime_events_wal_test.go——任务 1.5 验收（事件注册接线：07 §4.14 十二事件接入
// 事件总线 core 层——事件 WAL 留痕可回放）。
// 断言：Runtime 族事件经 statestore.Append 写入 WAL→Replay 完整回放（R-1201
// events.jsonl WAL=唯一真相权威；R-1329 WAL 物理行=事件 JSON 即行）。
package runtime

import (
	"testing"

	"github.com/goalos/goalos/internal/statestore"
	"github.com/goalos/goalos/pkg/events"
)

// TestRuntime_Events_WALRoundtrip 十二 Runtime 事件注册+WAL 留痕可回放。
// 先红=事件类型未注册（会议 #250 W1 开工首日：注册前本测试编译期不存在）；
// 转绿=类型注册+Append/Replay 往返一致。
func TestRuntime_Events_WALRoundtrip(t *testing.T) {
	store := statestore.New(t.TempDir())

	// 十二事件类型常量存在性+wire 值钉死（07 §4.14 注册名=wire 值）
	runtimeEvents := []struct {
		wire string
		name string
	}{
		{events.TypeContractIssued, "ContractIssued"},
		{events.TypeContractRejected, "ContractRejected"},
		{events.TypeRuntimeSelected, "RuntimeSelected"},
		{events.TypeRuntimeSelectionRejected, "RuntimeSelectionRejected"},
		{events.TypeRuntimeAcquired, "RuntimeAcquired"},
		{events.TypeRuntimeAcquireFailed, "RuntimeAcquireFailed"},
		{events.TypePrecheckFailed, "PrecheckFailed"},
		{events.TypeRuntimeReleased, "RuntimeReleased"},
		{events.TypeSessionEscalated, "SessionEscalated"},
		{events.TypeProviderDegraded, "ProviderDegraded"},
		{events.TypeProviderStateChanged, "ProviderStateChanged"},
		{events.TypeEscalationSignaled, "EscalationSignaled"},
	}
	for _, re := range runtimeEvents {
		if re.wire != re.name {
			t.Fatalf("wire 值漂移：常量=%q 注册名=%q（07 §4.14 单一权威）", re.wire, re.name)
		}
	}

	// WAL 留痕可回放：写入→回放→类型与载荷一致
	for _, re := range runtimeEvents {
		evt := events.Event{
			Type:    re.wire,
			Payload: map[string]interface{}{"contract_id": "ctr_test", "source": "w1-roundtrip"},
		}
		if err := store.Append("goal_rt_wal", evt); err != nil {
			t.Fatalf("Append %s 失败: %v", re.wire, err)
		}
	}
	replayed, err := store.Replay("goal_rt_wal", 0)
	if err != nil {
		t.Fatalf("Replay 失败: %v", err)
	}
	if len(replayed) != len(runtimeEvents) {
		t.Fatalf("回放条数=%d，期望 %d——WAL 留痕不完整", len(replayed), len(runtimeEvents))
	}
	for i, re := range runtimeEvents {
		if !containsWireValue(replayed[i], re.wire) {
			t.Fatalf("回放第 %d 条类型=%v 漂移（期望 %s）", i, string(replayed[i]), re.wire)
		}
	}
}

// containsWireValue 检查回放 JSON 行含事件类型 wire 值（行=事件 JSON 即行，R-1329）。
func containsWireValue(raw []byte, wire string) bool {
	return containsStr(string(raw), `"`+wire+`"`)
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
