// session.go——ExecutionSession（任务 3.3；规格=05 §X.6.6 实体关系矩阵：会话与 Action 1:1
// ——R-1504；升级=优雅取消（Interrupt 链）+新会话挂同卷+被中断 Action 重执行——R-1499；
// 卷一致性=挂载前校验+.tmp 清理——R-1529；重签失败=任务失败/旧会话清理失败=强制销毁
// +留痕——R-1525；升级信号=能力代理层检测——R-1509）。
package runtime

import (
	"context"
	"fmt"
	"sync"
)

// SessionState ExecutionSession 状态机（05 §X.6.6；显式赋值从 1 开始——D-13 零值非法）。
type SessionState int

const (
	// SessionPreparing 准备中（Attach 后 Start 前——值=1）
	SessionPreparing SessionState = iota + 1
	SessionRunning                // 运行中（Start+Precheck 完成——Execute 唯一合法态）
	SessionPaused                 // 已挂起
	SessionEscalating             // 升级评估中（Escalate 期间）
	SessionEscalated              // 已升级关闭（旧会话终态——新会话承接）
	SessionDestroyed              // 强制销毁（升级清理失败——不留热池）
	SessionClosed                 // 正常关闭（Release 完成）
)

// String 呈现值。
func (s SessionState) String() string {
	switch s {
	case SessionPreparing:
		return "preparing"
	case SessionRunning:
		return "running"
	case SessionPaused:
		return "paused"
	case SessionEscalating:
		return "escalating"
	case SessionEscalated:
		return "escalated"
	case SessionDestroyed:
		return "destroyed"
	case SessionClosed:
		return "closed"
	default:
		return "invalid"
	}
}

// ExecutionSession 执行会话（Action 1:1——粒度钉死 R-1504；句柄经 HandleGuard 守门——
// R-1620 契约表唯一执行点）。
type ExecutionSession struct {
	mu        sync.Mutex
	id        string
	goalID    string
	workspace WorkspaceRef
	state     SessionState
	handle    *HandleGuard

	// Action 台账（升级语义数据源——已完成产出物不重复，R-1499 断言③）
	completed  map[string][]string // actionID → 产出物清单
	executing  map[string]bool     // 执行中 actionID
	cancelled  map[string]bool     // 被中断标记 Cancelled（断言②）
	interrupted []string           // 待重执行队列（升级时被中断的 Action）
}

// NewExecutionSession 建会话（准备中——未挂句柄）。
func NewExecutionSession(id, goalID string, workspace WorkspaceRef) *ExecutionSession {
	return &ExecutionSession{
		id:         id,
		goalID:     goalID,
		workspace:  workspace,
		state:      SessionPreparing,
		completed:  make(map[string][]string),
		executing:  make(map[string]bool),
		cancelled:  make(map[string]bool),
	}
}

// Attach 挂接 Provider 句柄（Acquire→Start→Precheck 链——D-5 定序；HandleGuard 守门包装）。
func (s *ExecutionSession) Attach(ctx context.Context, p Provider, req LeaseRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != SessionPreparing {
		return fmt.Errorf("runtime: Attach 非法状态 %v（仅 preparing 可挂接）", s.state)
	}
	h, err := p.Acquire(ctx, req)
	if err != nil {
		return fmt.Errorf("runtime: Acquire 失败: %w", err)
	}
	guard := NewHandleGuard(h)
	if err := guard.Start(ctx); err != nil {
		return fmt.Errorf("runtime: Start 失败: %w", err)
	}
	if err := guard.Precheck(ctx); err != nil {
		return fmt.Errorf("runtime: Precheck 失败（边界建立未生效——句柄销毁非归还热池，RTM-PRECHECK-F-001 族）: %w", err)
	}
	s.handle = guard
	s.state = SessionRunning
	return nil
}

// State 会话状态。
func (s *ExecutionSession) State() SessionState { s.mu.Lock(); defer s.mu.Unlock(); return s.state }

// Workspace 工作区卷引用（升级=新会话挂同卷——R-1499）。
func (s *ExecutionSession) Workspace() WorkspaceRef { return s.workspace }

// Execute 执行 Action（仅 Running——R-1620）。
func (s *ExecutionSession) Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error) {
	s.mu.Lock()
	if s.state != SessionRunning {
		s.mu.Unlock()
		return ExecuteResult{}, fmt.Errorf("runtime: 会话非运行态 %v——Execute 非法", s.state)
	}
	s.executing[req.ActionID] = true
	h := s.handle
	s.mu.Unlock()

	res, err := h.Execute(ctx, req)

	s.mu.Lock()
	delete(s.executing, req.ActionID)
	s.mu.Unlock()
	return res, err
}

// MarkActionCompleted 登记 Action 完成+产出物清单（升级不重复产出断言数据源）。
func (s *ExecutionSession) MarkActionCompleted(actionID string, artifacts []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.completed[actionID] = artifacts
}

// MarkActionExecuting 登记 Action 执行中（升级时被中断=Cancelled 标记来源）。
func (s *ExecutionSession) MarkActionExecuting(actionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executing[actionID] = true
}

// ActionStatus Action 状态（Completed/Executing/Cancelled/未知）。
func (s *ExecutionSession) ActionStatus(actionID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancelled[actionID] {
		return "Cancelled"
	}
	if _, ok := s.completed[actionID]; ok {
		return "Completed"
	}
	if s.executing[actionID] {
		return "Executing"
	}
	return "Unknown"
}

// Escalate 升级（新会话挂同卷——R-1499 三断言：①Interrupt 链优雅取消②被中断 Action
// 标记 Cancelled③新会话重执行且不重复已完成产出）。
// signal=capability_proxy|risk_reeval（EscalationSignaled 枚举——R-1560）。
// 取消/清理失败=升级失败+旧会话强制销毁（不留热池——R-1525）。
func (s *ExecutionSession) Escalate(ctx context.Context, newProvider Provider, signal string) (*ExecutionSession, error) {
	s.mu.Lock()
	if s.state != SessionRunning {
		s.mu.Unlock()
		return nil, fmt.Errorf("runtime: 升级非法状态 %v（仅运行中可升级）", s.state)
	}
	s.state = SessionEscalating
	// 被中断 Action=执行中集合 → Cancelled+待重执行队列
	for id := range s.executing {
		s.cancelled[id] = true
		s.interrupted = append(s.interrupted, id)
	}
	handle := s.handle
	s.mu.Unlock()

	// ①优雅取消（Interrupt 链——CancelMessage→SIGTERM→2s→SIGKILL 归 Provider 实现）
	if handle != nil {
		if err := handle.Interrupt(ctx); err != nil {
			// 取消失败=升级失败；旧会话强制销毁+留痕（R-1525——不留热池）
			s.mu.Lock()
			s.state = SessionDestroyed
			s.mu.Unlock()
			return nil, fmt.Errorf("runtime: 升级优雅取消失败——旧会话强制销毁（留痕）: %w", err)
		}
		if err := handle.Release(ctx); err != nil {
			s.mu.Lock()
			s.state = SessionDestroyed
			s.mu.Unlock()
			return nil, fmt.Errorf("runtime: 旧会话清理失败——强制销毁+留痕（R-1525）: %w", err)
		}
	}

	// 新会话挂同卷
	newSess := NewExecutionSession(s.id+"-escalated", s.goalID, s.workspace)
	// 台账继承：已完成（重执行跳过判定）+被中断队列（重执行对象——R-1499 断言③）
	newSess.completed = s.completed
	newSess.interrupted = append([]string{}, s.interrupted...)

	s.mu.Lock()
	s.state = SessionEscalated
	s.mu.Unlock()

	// 新会话挂接新 Provider（新契约=治理重签——签发链归治理层，本层挂接）
	if err := newSess.Attach(ctx, newProvider, LeaseRequest{GoalID: s.goalID}); err != nil {
		return nil, fmt.Errorf("runtime: 新会话挂接失败（重签失败=任务失败——RTM-RESOLVE-F-001 族）: %w", err)
	}
	_ = signal // signal 入事件载荷（SessionEscalated.escalation_signal——daemon 接线随任务 3.3 事件发射点）
	return newSess, nil
}

// ReexecuteInterrupted 重执行升级时被中断的 Action（已完成 Action 跳过——产出物不重复，R-1499 断言③）。
func (s *ExecutionSession) ReexecuteInterrupted(ctx context.Context) error {
	s.mu.Lock()
	if s.state != SessionRunning {
		s.mu.Unlock()
		return fmt.Errorf("runtime: 重执行非法状态 %v", s.state)
	}
	queue := append([]string{}, s.interrupted...)
	completed := s.completed
	s.mu.Unlock()

	for _, actionID := range queue {
		if _, done := completed[actionID]; done {
			continue // 已完成=不重复（产出物不重复）
		}
		if _, err := s.Execute(ctx, ExecuteRequest{ActionID: actionID, ActionType: "reexecute"}); err != nil {
			return fmt.Errorf("runtime: 重执行 %s 失败: %w", actionID, err)
		}
	}
	return nil
}
