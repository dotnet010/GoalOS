// Package daemon 实现 GoalOS Daemon 的 HTTP API 端点。
//
// 设计依据：05 架构文档 §2.2（W1 API 12 端点）、R194。
package daemon

import (
	"encoding/json"
	"net/http"
	"os"
	"sync"

	goalErr "github.com/goalos/goalos/pkg/errors"
	"github.com/goalos/goalos/pkg/events"
)

// Handler 包含所有 HTTP 端点处理逻辑。
type Handler struct {
	Goals      map[string]*GoalRecord // W1: 内存存储。W3: State Store
	mu         sync.RWMutex
	onShutdown func() // 关闭回调。daemon main 设置
}

// SetShutdownHook 设置关闭回调。W4：由 daemon main 在启动时调用。
func (h *Handler) SetShutdownHook(fn func()) {
	h.onShutdown = fn
}

// GoalRecord 是 Goal 的 API 响应记录。
type GoalRecord struct {
	ID     string `json:"goal_id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// NewHandler 创建一个 API Handler。
func NewHandler() *Handler {
	return &Handler{
		Goals: make(map[string]*GoalRecord),
	}
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
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"pid":    os.Getpid(),
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

	// 发布 GoalCreated 事件到 Event Bus（由 daemon main 在启动时通过 SetEventBus 注入）
	if eventBus != nil {
		eventBus.Publish(events.NewEvent(events.TypeGoalCreated, goalID, "daemon").WithPayload(map[string]interface{}{
			"title":       body.Goal,
			"description": body.Goal,
		}))
	}

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
	writeJSON(w, http.StatusOK, goal)
}

// HandleListGoals 列出全部 Goal。GET /api/goals。
func (h *Handler) HandleListGoals(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	goals := make([]*GoalRecord, 0, len(h.Goals))
	for _, g := range h.Goals {
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

// HandleGoalLog 审计日志。GET /api/goals/:id/log。
func (h *Handler) HandleGoalLog(w http.ResponseWriter, r *http.Request) {
	goalID := r.PathValue("id")
	if goalID == "" {
		writeError(w, http.StatusBadRequest, goalErr.CodeInvalidRequest, "缺少 goal id")
		return
	}
	// W2: 返回空日志。W3: State Store 查询
	writeJSON(w, http.StatusOK, []map[string]string{})
}

// HandleGoalEvents 事件导出。GET /api/goals/:id/events。
func (h *Handler) HandleGoalEvents(w http.ResponseWriter, r *http.Request) {
	goalID := r.PathValue("id")
	if goalID == "" {
		writeError(w, http.StatusBadRequest, goalErr.CodeInvalidRequest, "缺少 goal id")
		return
	}
	// W2: 返回空事件流。W3: State Store 查询
	w.Header().Set("Content-Type", "application/x-jsonlines")
	w.WriteHeader(http.StatusOK)
}

// HandleDaemonStop 停止 Daemon。POST /api/system/stop。
func (h *Handler) HandleDaemonStop(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "stopping"})
	// 触发优雅关闭——由 main() 的 signal handler 处理
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
		"active_goals": len(h.Goals),
	})
}

// ─── 内部 ───

var goalCounter int

func generateGoalID() string {
	goalCounter++
	return "goal_" + padInt(goalCounter)
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

// eventBus 是注入的 EventBus（由 daemon main 在启动时设置）。
// W1: 通过全局变量注入。W3: 依赖注入。
var eventBus interface {
	Publish(events.Event)
}

// SetEventBus 注入 EventBus。
func SetEventBus(bus interface{ Publish(events.Event) }) {
	eventBus = bus
}
