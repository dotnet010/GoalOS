// 契约测试：状态代数+Truth 模型（R-1091~R-1096/R-1101/R-1104/R-1128——会议 #195~#197）。
//
// 断言来源: R-1091（CompletionContract 第一性原则）/R-1092（Logical/Physical Truth+
//   Commit Point）/R-1093（定期增量 Checkpoint）/R-1094（重做前置回滚幂等）/
//   R-1095（状态代数矩阵+Stopped 终态）/R-1096（审批权威归一）/R-1101（Guard/Drift/
//   Canary 决策序列）/R-1104（Crash Recovery 唯一算法）/R-1128（状态机编码规范）。
//
// 转绿说明（任务 3.26 完成态——单一权威定义 internal/governance/state_algebra.go）:
//   四维矩阵 GoalPhase×ActionPhase×PipelinePhase×ApprovalPhase 以类型承载
//   （R-1136 四维补全/R-1343 TimedOut 删除/R-1405 Cancelled 补全）；非法迁移=
//   ValidateTransition error + StateMachineViolation 发布（R-1362/R-1407）；
//   Commit Point=WAL append+fsync（R-1397——投影失败不回滚 WAL）。
//
// 契约 MUST:
//   - MUST 1（R-1095）: Stopped(user_stopped) 终态独立存在且不伪装 Failed。
//   - MUST 2（R-1407/R-1362）: 非法迁移被拒绝（error + StateMachineViolation 事件）。
//   - MUST 3（R-1397）: Commit Point=WAL append+fsync——投影（state.json）写失败
//     不回滚 WAL（WAL 已提交事件仍可回放）。
//
// 纪律: 行为断言——禁止源码文本断言；逐条 t.Error 而非 FailNow。

package governance_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/governance"
	"github.com/goalos/goalos/internal/statestore"
	"github.com/goalos/goalos/pkg/events"
)

// TestStateMatrix_StoppedTerminal_NotFailed — Stopped 终态不伪装 Failed（R-1095）。
// 断言：Stopped(user_stopped) 终态独立存在，与 Failed 终态值不同且同为终态。
func TestStateMatrix_StoppedTerminal_NotFailed(t *testing.T) {
	// MUST 1a: Stopped 是终态（用户主动停止=状态机终点）。
	if !governance.GoalStopped.Terminal() {
		t.Error("MUST 1（R-1095）: GoalStopped.Terminal()=false——user_stopped 必须为终态")
	}
	// MUST 1b: Failed 是终态。
	if !governance.GoalFailed.Terminal() {
		t.Error("MUST 1（R-1095）: GoalFailed.Terminal()=false——failed 必须为终态")
	}
	// MUST 1c: Stopped 与 Failed 值不同——Stopped 不伪装 Failed（R-1095）。
	if governance.GoalStopped == governance.GoalFailed {
		t.Error("MUST 1（R-1095）: GoalStopped==GoalFailed——Stopped(user_stopped) 伪装 Failed 违约")
	}
	// MUST 1d: 非终态不得误标终态（Running/Paused 仍可迁移）。
	if governance.GoalRunning.Terminal() || governance.GoalPaused.Terminal() {
		t.Error("MUST 1（R-1095）: Running/Paused 误标终态——矩阵终态判定过宽")
	}
}

// TestStateMatrix_IllegalTransitionRejected — 非法迁移拒绝（R-1407 交叉约束规则集）。
// 断言：交叉约束违反（终态后迁移/拒绝后完成/批准后仍 Pending）→ error +
// StateMachineViolation 事件（R-1362）；合法迁移→nil 且无事件。
func TestStateMatrix_IllegalTransitionRejected(t *testing.T) {
	bus := eventbus.New()
	gov := governance.New(bus, make([]byte, 32))
	violations := make(chan events.Event, 4)
	bus.Subscribe(events.TypeStateMachineViolation, func(evt events.Event) error {
		violations <- evt
		return nil
	})

	illegal := []struct {
		name string
		cur  governance.StateMatrix
		next governance.StateMatrix
	}{
		{
			"terminal_goal_reopens",
			governance.StateMatrix{Goal: governance.GoalCompleted},
			governance.StateMatrix{Goal: governance.GoalRunning},
		},
		{
			"terminal_goal_restarts_exec",
			governance.StateMatrix{Goal: governance.GoalStopped, Pipeline: governance.PipelineDecide},
			governance.StateMatrix{Goal: governance.GoalStopped, Pipeline: governance.PipelineExec},
		},
		{
			"rejected_claims_completed",
			governance.StateMatrix{Approval: governance.ApprovalRejected},
			governance.StateMatrix{Approval: governance.ApprovalRejected, Action: governance.ActionCompleted},
		},
		{
			"approved_stays_pending",
			governance.StateMatrix{Approval: governance.ApprovalApproved},
			governance.StateMatrix{Approval: governance.ApprovalApproved, Action: governance.ActionPending},
		},
	}
	for _, tc := range illegal {
		if err := tc.cur.ValidateTransition(tc.next); err == nil {
			t.Errorf("MUST 2（R-1407）: 非法迁移 %s 未被拒绝——交叉约束规则集未生效", tc.name)
		}
		if err := gov.RejectIllegalTransition("goal-illegal", tc.cur, tc.next); err == nil {
			t.Errorf("MUST 2（R-1362）: 非法迁移 %s 经 Engine 闸门未被拒绝", tc.name)
		}
	}
	// 四组非法迁移必须各发布一条 StateMachineViolation（R-1362）。
	for i := 0; i < len(illegal); i++ {
		select {
		case evt := <-violations:
			if evt.Type != events.TypeStateMachineViolation {
				t.Errorf("MUST 2（R-1362）: 违规事件类型=%s，必须为 %s", evt.Type, events.TypeStateMachineViolation)
			}
		default:
			t.Errorf("MUST 2（R-1362）: 第 %d 条非法迁移未发布 StateMachineViolation", i+1)
		}
	}

	// 合法迁移: Paused→Running（Goal 非终态迁移，失败 Action 重做）必须放行且无违规事件。
	// 注意避免规则 4（approved→pending）触发——合法样例以 Rejected 为当前 Approval。
	if err := gov.RejectIllegalTransition("goal-legal",
		governance.StateMatrix{Goal: governance.GoalPaused, Action: governance.ActionFailed, Pipeline: governance.PipelineDecide, Approval: governance.ApprovalRejected},
		governance.StateMatrix{Goal: governance.GoalRunning, Action: governance.ActionPending, Pipeline: governance.PipelineCheck, Approval: governance.ApprovalPending},
	); err != nil {
		t.Errorf("MUST 2（R-1407）: 合法迁移被误拒: %v", err)
	}
	select {
	case evt := <-violations:
		t.Errorf("MUST 2（R-1362）: 合法迁移误发 StateMachineViolation（%s）", evt.Type)
	default:
		// 无违规事件=正确
	}
}

// TestCommitPoint_WALCommitOnly — Commit Point=WAL append+fsync 完成（R-1397）。
// 断言：WAL 落盘即对外可见；投影（state.json）写失败=投影落后，不回滚 WAL
// ——已提交事件仍可回放（bbolt 投影=滞后副作用，R-1109/R-1397）。
func TestCommitPoint_WALCommitOnly(t *testing.T) {
	baseDir := t.TempDir()
	store := statestore.New(baseDir)
	goalID := "goal-commit-pt"

	// Commit Point: WAL append+fsync 成功。
	evt := events.Event{Type: events.TypeActionStarted, GoalID: goalID, Source: "test"}
	if err := store.Append(goalID, evt); err != nil {
		t.Fatalf("前置: WAL append 失败: %v", err)
	}

	// 投影写失败模拟: state.json 路径被目录占用——SaveState 必然失败。
	dir := filepath.Join(baseDir, goalID)
	if err := os.MkdirAll(filepath.Join(dir, "state.json"), 0755); err != nil {
		t.Fatalf("前置: 投影路径占位失败: %v", err)
	}
	if err := store.SaveState(goalID, &statestore.GoalState{GoalID: goalID, InternalState: "running"}); err == nil {
		t.Fatal("前置: SaveState 应失败（state.json 为目录）——占位无效")
	}

	// MUST 3a: 投影失败不回滚 WAL——已提交事件仍可回放（Commit Point 独立于投影）。
	events2, err := store.Replay(goalID, 0)
	if err != nil {
		t.Fatalf("MUST 3（R-1397）: 投影失败后 Replay 报错: %v", err)
	}
	if len(events2) != 1 {
		t.Errorf("MUST 3（R-1397）: 投影失败后 WAL 事件数=%d，必须为 1——投影失败不得回滚 WAL", len(events2))
	}

	// MUST 3b: 回放内容与提交事件一致（type+goal_id 保真）。
	// Replay 返回完整 WAL 行（含 \t<crc>\t<version> envelope——R-1453）——
	// 读取方契约：先按 \t 拆分取 JSON 部分。
	jsonPart := events2[0]
	if idx := bytes.IndexByte(jsonPart, '\t'); idx >= 0 {
		jsonPart = jsonPart[:idx]
	}
	var replayed events.Event
	if err := json.Unmarshal(jsonPart, &replayed); err != nil {
		t.Fatalf("MUST 3（R-1397）: 回放解码失败: %v", err)
	}
	if replayed.Type != evt.Type || replayed.GoalID != goalID {
		t.Errorf("MUST 3（R-1397）: 回放事件失真——type=%s goal=%s，期望 %s/%s", replayed.Type, replayed.GoalID, evt.Type, goalID)
	}
}
