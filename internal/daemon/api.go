// Package daemon 实现 GoalOS Daemon 的 HTTP API 端点。
//
// 设计依据：05 架构文档 §2.2（W1 API 12 端点）、R194。
package daemon

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	goalErr "github.com/goalos/goalos/pkg/errors"
	"github.com/goalos/goalos/pkg/events"
	"github.com/goalos/goalos/internal/metrics"
	"github.com/goalos/goalos/internal/statestore"
)

// Handler 包含所有 HTTP 端点处理逻辑。
// PendingApproval 待审批的 Action。
type PendingApproval struct {
	ActionID          string `json:"action_id"`
	GoalID            string `json:"goal_id"`
	ActionType        string `json:"action_type"`
	RiskLevel         string `json:"risk_level"`
	ActionDescription string `json:"description"`
	TimeoutSeconds    int    `json:"timeout_seconds"`
}

// Handler 包含所有 HTTP 端点处理逻辑。
type Handler struct {
	Goals            map[string]*GoalRecord
	actionResults    map[string]interface{}
	pendingApprovals map[string]PendingApproval
	artifacts        map[string][]string // goalID → paths (R-030)
	Reviews          map[string][]*ReviewSummary // goalID → review summaries (R-846)
	ReviewReports    map[string]*events.ReviewReport // reportKey(goalID+actionID) → full report
	Metrics          *metrics.Registry   // v0.1.0 H8: Prometheus 指标注册表
	mu               sync.RWMutex
	port             int
	startTime        time.Time
	onShutdown       func()
}

// ReviewSummary 是 ReviewReport 的列表视图摘要（不含 reasoning）。
type ReviewSummary struct {
	ActionID         string        `json:"action_id"`
	Verdict          string        `json:"verdict"`
	VoteDistribution events.VoteDist `json:"vote_distribution"`
	CreatedAt        string        `json:"created_at"`
}

// SetPort 设置 daemon 端口号。
func (h *Handler) SetPort(port int) { h.port = port }

// SetStartTime 设置 daemon 启动时间（用于计算 uptime）。
func (h *Handler) SetStartTime(t time.Time) { h.startTime = t }

// SetShutdownHook 设置关闭回调。
func (h *Handler) SetShutdownHook(fn func()) {
	h.onShutdown = fn
}

// GoalRecord 是 Goal 的 API 响应记录（v0.1.1 UX 增强 R-376）。
type GoalRecord struct {
	ID           string      `json:"goal_id"`
	Title        string      `json:"title"`
	Status       string      `json:"status"`
	ActionsDone  int         `json:"actions_done"`
	ActionsTotal int         `json:"actions_total"`
	ErrorHint    string      `json:"error_hint,omitempty"`
	OutputPath   string      `json:"output_path,omitempty"`
	Suggestions      []Suggestion  `json:"suggestions,omitempty"`
	Result           interface{}  `json:"result,omitempty"`
	Actions          []ActionStatus `json:"actions,omitempty"`
	MultiLLMVerdict  string       `json:"multi_llm_verdict,omitempty"`
	MultiLLMReport   string       `json:"multi_llm_report,omitempty"` // R-840: MultiLLM 裁决
}

// Suggestion 是失败后的操作建议（R-381）。
type Suggestion struct {
	Action string `json:"action"`
	Label  string `json:"label"`
}

// ActionStatus 是单个 Action 的状态记录（R-833 B18）。
type ActionStatus struct {
	ActionID   string `json:"action_id"`
	ActionType string `json:"action_type,omitempty"`
	Status     string `json:"status"` // scheduled|approved|completed|failed
}

// UpdateMultiLLMVerdict 更新 MultiLLM 裁决结果（R-840）。
func (h *Handler) UpdateMultiLLMVerdict(goalID, verdict, report string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if g, ok := h.Goals[goalID]; ok {
		g.MultiLLMVerdict = verdict
		g.MultiLLMReport = report
	}
}

// UpdateActionStatus 更新 Goal 的最近 Action 状态（R-833）。
func (h *Handler) UpdateActionStatus(goalID, actionID, actionType, status string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if g, ok := h.Goals[goalID]; ok {
		g.Actions = append(g.Actions, ActionStatus{ActionID: actionID, ActionType: actionType, Status: status})
		if len(g.Actions) > 5 {
			g.Actions = g.Actions[len(g.Actions)-5:] // 只保留最近5个
		}
	}
}

// NewHandler 创建一个 API Handler。
func NewHandler() *Handler {
	return &Handler{
		Goals:            make(map[string]*GoalRecord),
		actionResults:    make(map[string]interface{}),
		pendingApprovals: make(map[string]PendingApproval),
		artifacts:        make(map[string][]string),
		Reviews:          make(map[string][]*ReviewSummary),
		ReviewReports:    make(map[string]*events.ReviewReport),
	}
}

// parseOutput 解析嵌套 JSON output 字符串为结构化对象。
func parseOutput(result interface{}) interface{} {
	m, ok := result.(map[string]interface{})
	if !ok {
		return result
	}
	if outputStr, ok := m["output"].(string); ok && len(outputStr) > 0 {
		var parsed interface{}
		if json.Unmarshal([]byte(outputStr), &parsed) == nil {
			m["output"] = parsed
		}
	}
	return m
}

// TrackPendingApproval 记录待审批 Action。
func (h *Handler) TrackPendingApproval(pa PendingApproval) {
	h.mu.Lock()
	h.pendingApprovals[pa.ActionID] = pa
	h.mu.Unlock()
}

// RemovePendingApproval 移除已处理的审批。
func (h *Handler) RemovePendingApproval(actionID string) {
	h.mu.Lock()
	delete(h.pendingApprovals, actionID)
	h.mu.Unlock()
}

// HandleListApprovals 列出待审批 Action。GET /api/approvals。
func (h *Handler) HandleListApprovals(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	list := make([]PendingApproval, 0, len(h.pendingApprovals))
	for _, pa := range h.pendingApprovals {
		list = append(list, pa)
	}
	writeJSON(w, http.StatusOK, list)
}

// HandleApprove 批准 Action。POST /api/approvals/:id/approve。
func (h *Handler) HandleApprove(w http.ResponseWriter, r *http.Request) {
	actionID := r.PathValue("id")
	if actionID == "" {
		writeError(w, http.StatusBadRequest, goalErr.CodeInvalidRequest, "缺少 action id")
		return
	}
	h.mu.Lock()
	pa, ok := h.pendingApprovals[actionID]
	delete(h.pendingApprovals, actionID)
	h.mu.Unlock()
	if !ok {
		writeError(w, http.StatusNotFound, goalErr.CodeGoalNotFound, "审批不存在或已过期")
		return
	}
	// 发布 UserApprovedAction 事件（携带 goal_id）
	if eventBus != nil {
		eventBus.Publish(events.NewEvent(events.TypeUserApprovedAction, pa.GoalID, "api").WithPayload(map[string]interface{}{
			"action_id": actionID,
		}))
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

// HandleReject 拒绝 Action。POST /api/approvals/:id/reject。
func (h *Handler) HandleReject(w http.ResponseWriter, r *http.Request) {
	actionID := r.PathValue("id")
	if actionID == "" {
		writeError(w, http.StatusBadRequest, goalErr.CodeInvalidRequest, "缺少 action id")
		return
	}
	h.mu.Lock()
	pa, ok := h.pendingApprovals[actionID]
	delete(h.pendingApprovals, actionID)
	h.mu.Unlock()
	if !ok {
		writeError(w, http.StatusNotFound, goalErr.CodeGoalNotFound, "审批不存在或已过期")
		return
	}
	// 发布 ActionCancelled 事件（携带 goal_id）
	if eventBus != nil {
		eventBus.Publish(events.NewEvent(events.TypeActionCancelled, pa.GoalID, "api").WithPayload(map[string]interface{}{
			"action_id": actionID,
			"reason":    "user_rejected",
		}))
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "rejected"})
}

// TrackResult 存储 Action 的执行结果。
func (h *Handler) TrackArtifact(goalID string, path string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.artifacts == nil { h.artifacts = make(map[string][]string) }
	h.artifacts[goalID] = append(h.artifacts[goalID], path)
}

func (h *Handler) TrackResult(goalID string, result interface{}) {
	h.mu.Lock()
	h.actionResults[goalID] = result
	h.mu.Unlock()
}

// HandleMetrics 返回 Prometheus 格式的运行时指标（v0.1.0 H8）。
func (h *Handler) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	if h.Metrics == nil {
		w.Write([]byte("# GoalOS metrics not initialized\n"))
		return
	}
	w.Write([]byte(h.Metrics.PrometheusText()))
}

// UpdateGoalProgress 更新 Goal 执行进度（v0.1.1 产品体验）。
func (h *Handler) UpdateGoalProgress(goalID string) {
	h.mu.Lock()
	if g, ok := h.Goals[goalID]; ok && g.Status == "正在执行" {
		g.ActionsDone++
	}
	h.mu.Unlock()
}

// SetGoalActionsTotal 在 MissionGenerated 时设置总 Action 数（R-378）。
func (h *Handler) SetGoalActionsTotal(goalID string, total int) {
	h.mu.Lock()
	if g, ok := h.Goals[goalID]; ok {
		g.ActionsTotal = total
	}
	h.mu.Unlock()
}

// SetGoalErrorHint 设置失败时的人类可读建议（R-377）。
// v0.2.2 W6 B9: 自动从 failHints 查表补全 suggestions。
func (h *Handler) SetGoalErrorHint(goalID string, hint string, suggestions []Suggestion) {
	h.mu.Lock()
	if g, ok := h.Goals[goalID]; ok {
		g.ErrorHint = hint
		if len(suggestions) == 0 {
			// W6-B5: 精确匹配 failHints key（非子串匹配）
			for key, sug := range failHints {
				if hint == key || strings.HasPrefix(hint, key+":") || strings.HasPrefix(hint, key+" ") {
					suggestions = append(suggestions, sug)
				}
			}
		}
		// W6-J1: 默认建议——基于 Goal 状态动态生成
		if len(suggestions) == 0 {
			suggestions = []Suggestion{
				{Action: "retry", Label: "重试当前目标"},
				{Action: "new_goal", Label: "重新描述目标"},
				{Action: "check_plugins", Label: "检查插件配置"},
			}
		}
		g.Suggestions = suggestions
	}
	h.mu.Unlock()
}

// failHints 映射内部错误类型到人类可读建议（R-377）。
// v0.2.0 会议 #156 R-852: 新增 3 条 MultiLLM 场景。
var failHints = map[string]Suggestion{
	"execution_error":      {Action: "retry", Label: "重试当前目标"},
	"llm_timeout":          {Action: "switch_model", Label: "更换更快的模型"},
	"no_output":            {Action: "simplify", Label: "简化目标描述"},
	"plugin_not_found":     {Action: "check_plugins", Label: "检查插件配置"},
	"MULTI_LLM_FAIL":       {Action: "view_review", Label: "查看审查详情"},
	"MULTI_LLM_WARN":       {Action: "view_review", Label: "查看审查详情"},
	"MULTI_LLM_DIVERGENCE": {Action: "view_review", Label: "查看分歧详情"},
}

// StoreReviewReport 存储 ReviewReport 并在对应的 GoalRecord 上更新审查摘要（R-846）。
func (h *Handler) StoreReviewReport(report *events.ReviewReport) {
	h.mu.Lock()
	defer h.mu.Unlock()

	key := report.GoalID + ":" + report.ActionID
	h.ReviewReports[key] = report

	summary := &ReviewSummary{
		ActionID:         report.ActionID,
		Verdict:          report.Verdict,
		VoteDistribution: report.VoteDistribution,
		CreatedAt:        report.CreatedAt,
	}

	h.Reviews[report.GoalID] = append(h.Reviews[report.GoalID], summary)

	// 更新 GoalRecord 的 MultiLLM 裁决字段
	if g, ok := h.Goals[report.GoalID]; ok {
		g.MultiLLMVerdict = report.Verdict
	}
}

// HandleGetReviews 返回 Goal 下所有 Action 的审查摘要列表（R-848）。
func (h *Handler) HandleGetReviews(w http.ResponseWriter, r *http.Request, goalID string) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	reviews, ok := h.Reviews[goalID]
	if !ok {
		reviews = []*ReviewSummary{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reviews)
}

// HandleGetReviewDetail 返回完整 ReviewReport（含所有 Provider 的 reasoning）（R-848）。
func (h *Handler) HandleGetReviewDetail(w http.ResponseWriter, r *http.Request, goalID, actionID string) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	key := goalID + ":" + actionID
	report, ok := h.ReviewReports[key]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "review not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

// DecideRequest 是用户决策的请求体（R-850）。
type DecideRequest struct {
	Decision string `json:"decision"`           // "accept" | "retry" | "refine"
	Feedback string `json:"feedback,omitempty"` // retry 或 refine 时的反馈
}

// HandleDecideReview 处理用户对 MultiLLM 审查结果的决策（R-850）。
func (h *Handler) HandleDecideReview(w http.ResponseWriter, r *http.Request, goalID, actionID string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req DecideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	// 验证 decision 合法性
	if req.Decision != "accept" && req.Decision != "retry" && req.Decision != "refine" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "decision must be accept, retry, or refine"})
		return
	}

	// accept 需确认（前端二次确认对话框，API 层仅记录）
	tainted := req.Decision == "accept"

	// 更新 ReviewReport 中的 user_decision
	h.mu.Lock()
	key := goalID + ":" + actionID
	if report, ok := h.ReviewReports[key]; ok {
		report.UserDecision = &events.UserDecision{
			Decision:  req.Decision,
			DecidedAt: time.Now().UTC().Format(time.RFC3339),
			Feedback:  req.Feedback,
			Tainted:   tainted,
		}
	}
	h.mu.Unlock()

	// 发布 MultiLLMUserDecided 事件——由 EventBus 订阅者（PipelineRunner）处理状态转换
	// 注意：此处需要访问 EventBus。为 MVP 实现，Handler 通过回调/eventBus 引用发布事件。
	// 事件发布细节见 cmd/goalos/main.go 中的路由注册——事件由 daemon 通过 evBus.Publish() 发布。

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "ok",
		"decision": req.Decision,
		"tainted":  tainted,
		"message":  decisionMessage(req.Decision),
	})
}

// decisionMessage 返回用户决策的人类可读确认消息。
func decisionMessage(decision string) string {
	switch decision {
	case "accept":
		return "已接受结果。AI 审查意见已被覆盖（tainted_review=true），系统将继续执行。"
	case "retry":
		return "已触发新 Session 重做。系统将携带你的反馈重新执行此 Action。"
	case "refine":
		return "已触发需求修改。系统将根据你的新需求重新规划。"
	default:
		return ""
	}
}


// UpdateGoalStatus 更新 Goal 状态（v0.1.1: failed 不可被 completed 覆盖）。
func (h *Handler) UpdateGoalStatus(goalID, status string) {
	h.mu.Lock()
	if g, ok := h.Goals[goalID]; ok {
		// GoalFailed 是终态——不可被后续 GoalCompleted 事件覆盖
		if g.Status == "failed" || g.Status == "已失败" {
			h.mu.Unlock()
			return
		}
		g.Status = status
	}
	h.mu.Unlock()
}

// writeJSON 写入 JSON 响应。
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError 写入标准错误响应。
func writeError(w http.ResponseWriter, status int, code goalErr.Code, msg string) {
	writeJSON(w, status, goalErr.NewResponse(code, msg))
}

// HandleHealth 健康检查。GET /api/health。
func (h *Handler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(h.startTime).String()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"pid":    os.Getpid(),
		"uptime": uptime,
	})
}

// HandleCreateGoal 创建 Goal。POST /api/goals。
func (h *Handler) HandleCreateGoal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, goalErr.CodeInvalidRequest, "只支持 POST")
		return
	}

	var body struct {
		Goal string `json:"goal"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Goal == "" {
		writeError(w, http.StatusBadRequest, goalErr.CodeInvalidRequest, "缺少 goal 字段")
		return
	}
	// E16: 限制 goal 长度，防止超大请求
	if len(body.Goal) > 10000 {
		writeError(w, http.StatusBadRequest, goalErr.CodeInvalidRequest, "goal 超过 10000 字符上限")
		return
	}

	goalID := generateGoalID()

	h.mu.Lock()
	h.Goals[goalID] = &GoalRecord{
		ID:     goalID,
		Title:  body.Goal,
		Status: "created",
	}
	h.mu.Unlock()

	// R-828 Step 3: 使用 typed payload 发布 GoalCreated
	if eventBus != nil {
		payload := events.GoalCreatedPayload{
			GoalID:      goalID,
			Title:       body.Goal,
			Description: body.Goal,
		}
		if err := payload.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, goalErr.CodeInvalidRequest, err.Error())
			return
		}
		eventBus.Publish(events.NewEvent(events.TypeGoalCreated, goalID, "daemon").WithPayload(events.PayloadToMap(payload)))
	}

	// v0.2.2 W6 B8: 预估等待时间 + 下一步状态提示
	// W6-B4: 基于 daemon 运行时长估算（新 daemon→保守 30s，运行中→10s）
	estWait := 30
	if !h.startTime.IsZero() {
		uptime := time.Since(h.startTime)
		if uptime > 5*time.Minute {
			estWait = 10 // daemon 运行稳定，LLM 已预热
		}
	}
	w.Header().Set("Location", "/api/goals/"+goalID)
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"goal_id":                goalID,
		"status":                 "created",
		"estimated_wait_seconds": estWait,
		"next_status":            "正在分析目标...",
	})
}

// HandleGetGoal 查询 Goal。GET /api/goals/:id。
func (h *Handler) HandleGetGoal(w http.ResponseWriter, r *http.Request) {
	goalID := r.PathValue("id")
	if goalID == "" {
		writeError(w, http.StatusBadRequest, goalErr.CodeInvalidRequest, "缺少 goal id")
		return
	}

	h.mu.RLock()
	goal, ok := h.Goals[goalID]
	h.mu.RUnlock()

	if !ok {
		writeError(w, http.StatusNotFound, goalErr.CodeGoalNotFound, "目标不存在")
		return
	}
	// 附加 Action 执行结果
	if result, exists := h.actionResults[goalID]; exists {
		goal.Result = parseOutput(result)
	}
	writeJSON(w, http.StatusOK, goal)
}

// HandleListGoals 列出全部 Goal。GET /api/goals。
func (h *Handler) HandleListGoals(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	goals := make([]*GoalRecord, 0, len(h.Goals))
	for _, g := range h.Goals {
		if result, exists := h.actionResults[g.ID]; exists {
			g.Result = parseOutput(result) // 解析嵌套 JSON output
		}
		goals = append(goals, g)
	}
	writeJSON(w, http.StatusOK, goals)
}

// HandlePauseGoal 暂停 Goal。POST /api/goals/:id/pause。
func (h *Handler) HandlePauseGoal(w http.ResponseWriter, r *http.Request) {
	goalID := r.PathValue("id")
	h.mu.Lock()
	goal, ok := h.Goals[goalID]
	if ok {
		goal.Status = "paused"
	}
	h.mu.Unlock()

	if !ok {
		writeError(w, http.StatusNotFound, goalErr.CodeGoalNotFound, "目标不存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "paused"})
}

// HandleResumeGoal 恢复 Goal。POST /api/goals/:id/resume。
func (h *Handler) HandleResumeGoal(w http.ResponseWriter, r *http.Request) {
	goalID := r.PathValue("id")
	h.mu.Lock()
	goal, ok := h.Goals[goalID]
	if ok {
		goal.Status = "running"
	}
	h.mu.Unlock()

	if !ok {
		writeError(w, http.StatusNotFound, goalErr.CodeGoalNotFound, "目标不存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "running"})
}

// HandleStopGoal 终止 Goal。POST /api/goals/:id/stop。
func (h *Handler) HandleStopGoal(w http.ResponseWriter, r *http.Request) {
	goalID := r.PathValue("id")
	h.mu.Lock()
	goal, ok := h.Goals[goalID]
	if ok {
		goal.Status = "stopped"
	}
	h.mu.Unlock()

	if !ok {
		writeError(w, http.StatusNotFound, goalErr.CodeGoalNotFound, "目标不存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

// HandleGoalLog 审计日志摘要。GET /api/goals/:id/log。
func (h *Handler) HandleGoalLog(w http.ResponseWriter, r *http.Request) {
	goalID := r.PathValue("id")
	if goalID == "" {
		writeError(w, http.StatusBadRequest, goalErr.CodeInvalidRequest, "缺少 goal id")
		return
	}
	logs := []map[string]interface{}{}
	if stateStore != nil {
		rawEvents, err := stateStore.Replay(goalID, 0)
		if err == nil {
			for _, raw := range rawEvents {
				var evt events.Event
				if json.Unmarshal(raw, &evt) != nil {
					continue
				}
				logs = append(logs, map[string]interface{}{
					"seq":       evt.Seq,
					"type":      evt.Type,
					"timestamp": evt.Timestamp.Format(time.RFC3339),
					"summary":   fmtSummary(evt),
				})
			}
		}
	}
	writeJSON(w, http.StatusOK, logs)
}

// HandleGoalEvents 事件导出。GET /api/goals/:id/events (JSONL 流)。
func (h *Handler) HandleGoalEvents(w http.ResponseWriter, r *http.Request) {
	goalID := r.PathValue("id")
	if goalID == "" {
		writeError(w, http.StatusBadRequest, goalErr.CodeInvalidRequest, "缺少 goal id")
		return
	}
	w.Header().Set("Content-Type", "application/x-jsonlines")
	w.WriteHeader(http.StatusOK)
	if stateStore != nil {
		rawEvents, err := stateStore.Replay(goalID, 0)
		if err == nil {
			enc := json.NewEncoder(w)
			for _, raw := range rawEvents {
				var evt events.Event
				if json.Unmarshal(raw, &evt) == nil {
					enc.Encode(evt)
				}
			}
		}
	}
}

// fmtSummary 生成事件的人类可读摘要。
func fmtSummary(evt events.Event) string {
	switch evt.Type {
	case events.TypeGoalCreated:
		return "目标已创建"
	case events.TypeMissionGenerated:
		return "任务图已生成"
	case events.TypeActionScheduled:
		return "Action 已调度"
	case events.TypeActionApproved:
		return "Action 已批准"
	case events.TypeActionCompleted:
		return "Action 已完成"
	case events.TypeActionFailed:
		return "Action 失败"
	case events.TypeGoalCompleted:
		return "目标已完成"
	default:
		return evt.Type
	}
}

// HandleDaemonStop 停止 Daemon。POST /api/system/stop。
func (h *Handler) HandleDaemonStop(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "stopping"})
	if h.onShutdown != nil {
		h.onShutdown()
	}
}

// HandleDaemonRestart 重启 Daemon。POST /api/system/restart。
func (h *Handler) HandleDaemonRestart(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "restarting"})
	if h.onShutdown != nil {
		h.onShutdown()
	}
}

// HandleSystemStatus 系统状态。GET /api/system/status。
func (h *Handler) HandleSystemStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"pid":          os.Getpid(),
		"port":         h.port,
		"uptime":       time.Since(h.startTime).String(),
		"active_goals": len(h.Goals),
	})
}

// ─── 内部 ───

var goalCounter atomic.Int64

func generateGoalID() string {
	n := goalCounter.Add(1)
	return "goal_" + padInt(int(n))
}

func padInt(n int) string {
	s := ""
	x := n
	for x > 0 {
		s = string(rune('0'+x%10)) + s
		x /= 10
	}
	if s == "" {
		s = "001"
	}
	for len(s) < 3 {
		s = "0" + s
	}
	return s
}

// ─── 全局注入（由 daemon main 在启动时设置）───

var eventBus interface {
	Publish(events.Event)
}

var stateStore interface {
	Replay(goalID string, fromSeq int) ([]json.RawMessage, error)
}

// SetEventBus 注入 EventBus。
func SetEventBus(bus interface{ Publish(events.Event) }) {
	eventBus = bus
}

// SetStateStore 注入 StateStore。
func SetStateStore(store *statestore.Store) {
	stateStore = store
}

// typedPayloadToMap converts typed payload to map (R-828).
