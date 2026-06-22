// Package daemon 实现 GoalOS Daemon 的 HTTP API 端点。
//
// 设计依据：05 架构文档 §2.2（W1 API 12 端点）、R194。
package daemon

import (
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	goalErr "github.com/goalos/goalos/pkg/errors"
	"github.com/goalos/goalos/pkg/events"
	"github.com/goalos/goalos/internal/statestore"
)

// Handler 包含所有 HTTP 端点处理逻辑。
type Handler struct {
	Goals       map[string]*GoalRecord
	actionResults map[string]interface{} // goalID → last action result
	mu          sync.RWMutex
	port        int
	startTime   time.Time
	onShutdown  func()
}

// SetPort 设置 daemon 端口号。
func (h *Handler) SetPort(port int) { h.port = port }

// SetStartTime 设置 daemon 启动时间（用于计算 uptime）。
func (h *Handler) SetStartTime(t time.Time) { h.startTime = t }

// SetShutdownHook 设置关闭回调。
func (h *Handler) SetShutdownHook(fn func()) {
	h.onShutdown = fn
}

// GoalRecord 是 Goal 的 API 响应记录。
type GoalRecord struct {
	ID     string `json:"goal_id"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Result interface{} `json:"result,omitempty"` // Goal 完成后的执行结果
}

// NewHandler 创建一个 API Handler。
func NewHandler() *Handler {
	return &Handler{
		Goals:         make(map[string]*GoalRecord),
		actionResults: make(map[string]interface{}),
	}
}

// TrackResult 存储 Action 的执行结果。
func (h *Handler) TrackResult(goalID string, result interface{}) {
	h.mu.Lock()
	h.actionResults[goalID] = result
	h.mu.Unlock()
}

// UpdateGoalStatus 更新 Goal 状态（GoalCompleted 时由 EventBus subscriber 调用）。
func (h *Handler) UpdateGoalStatus(goalID, status string) {
	h.mu.Lock()
	if g, ok := h.Goals[goalID]; ok {
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

	goalID := generateGoalID()

	h.mu.Lock()
	h.Goals[goalID] = &GoalRecord{
		ID:     goalID,
		Title:  body.Goal,
		Status: "created",
	}
	h.mu.Unlock()

	// 发布 GoalCreated 事件
	if eventBus != nil {
		eventBus.Publish(events.NewEvent(events.TypeGoalCreated, goalID, "daemon").WithPayload(map[string]interface{}{
			"title":       body.Goal,
			"description": body.Goal,
		}))
	}

	w.Header().Set("Location", "/api/goals/"+goalID)
	writeJSON(w, http.StatusCreated, map[string]string{
		"goal_id": goalID,
		"status":  "created",
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
	// 附加 Action 执行结果，解析嵌套 JSON output
	if result, exists := h.actionResults[goalID]; exists {
		if m, ok := result.(map[string]interface{}); ok {
			if outputStr, ok := m["output"].(string); ok && len(outputStr) > 0 {
				var parsed interface{}
				if json.Unmarshal([]byte(outputStr), &parsed) == nil {
					m["output"] = parsed // 替换为结构化对象
				}
			}
		}
		goal.Result = result
	}
	writeJSON(w, http.StatusOK, goal)
}

// HandleListGoals 列出全部 Goal。GET /api/goals。
func (h *Handler) HandleListGoals(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	goals := make([]*GoalRecord, 0, len(h.Goals))
	for _, g := range h.Goals {
		// Merge action result into goal record
		if result, exists := h.actionResults[g.ID]; exists {
			g.Result = result
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
