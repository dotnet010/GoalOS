// Package client implements the GoalOS daemon HTTP client.
// Used by CLI and Web UI. All state is in the daemon — client is stateless.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

const defaultTimeout = 30 * time.Second

// Client communicates with the GoalOS daemon via HTTP.
type Client struct {
	baseURL string
	http    *http.Client
}

// New creates a new Client.
// NewUDS UDS 客户端（治理面唯一通道——R-1378/R-1322：审批族端点 UDS-only；
// D-4 配套 R-1645——决策命令族实装的传输前提）。
// R-1246 transport×认证矩阵：本机治理命令走 unix socket。
func NewUDS(socketPath string) *Client {
	return &Client{
		baseURL: "http://goalos-daemon.uds", // 占位主机名（UDS 拨号不使用 Host 连接——仅 URL 形态需要）
		http: &http.Client{
			Timeout: defaultTimeout,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "unix", socketPath)
				},
			},
		},
	}
}

func New(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: defaultTimeout},
	}
}

// CLI 退出码契约（R-1323/R-1379）: 124=超时；125=审批未获通过/guard 拦截。
// 语义互斥——124 与 125 不可复用同一码位。
const (
	ExitCodeTimeout        = 124 // 超时（R-1323）
	ExitCodeApprovalDenied = 125 // 审批未获通过 / guard 拦截（R-1379）
)

// ExitCodeFor 将失败原因映射为 CLI 退出码（R-1323/R-1379）。
// 契约: cause="timeout"→124；"approval_denied"/"guard_intercepted"→125；
// 其余→1（通用失败——CLI 既有行为）。
func (c *Client) ExitCodeFor(cause string) int {
	switch cause {
	case "timeout":
		return ExitCodeTimeout
	case "approval_denied", "guard_intercepted":
		return ExitCodeApprovalDenied
	default:
		return 1
	}
}

// Health checks if the daemon is running.
func (c *Client) Health() (bool, error) {
	resp, err := c.http.Get(c.baseURL + "/api/health")
	if err != nil {
		return false, fmt.Errorf("client: health check: %w", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK, nil
}

// CreateGoalResponse is the response from POST /api/goals.
type CreateGoalResponse struct {
	GoalID string `json:"goal_id"`
	Status string `json:"status"`
	MultiLLMVerdict string `json:"multi_llm_verdict,omitempty"`
	MultiLLMReport  string `json:"multi_llm_report,omitempty"`
}

// GoalRecord is a single goal in list/get responses.
type GoalRecord struct {
	GoalID string `json:"goal_id"`
	Title  string `json:"title"`
	Status string `json:"status"`
	MultiLLMVerdict string `json:"multi_llm_verdict,omitempty"`
	MultiLLMReport  string `json:"multi_llm_report,omitempty"`
}

// CreateGoal sends a new Goal to the daemon.
func (c *Client) CreateGoal(goalText string) (*CreateGoalResponse, error) {
	body := struct {
		Goal string `json:"goal"`
	}{Goal: goalText}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("client: marshal goal: %w", err)
	}

	resp, err := c.http.Post(c.baseURL+"/api/goals", "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("client: create goal: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("client: create goal failed (%d): %s", resp.StatusCode, string(b))
	}

	var result CreateGoalResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("client: decode response: %w", err)
	}
	return &result, nil
}

// ListGoals returns all goals. GET /api/goals.
func (c *Client) ListGoals() ([]GoalRecord, error) {
	resp, err := c.http.Get(c.baseURL + "/api/goals")
	if err != nil {
		return nil, fmt.Errorf("client: list goals: %w", err)
	}
	defer resp.Body.Close()
	var goals []GoalRecord
	if err := json.NewDecoder(resp.Body).Decode(&goals); err != nil {
		return nil, fmt.Errorf("client: decode goals: %w", err)
	}
	return goals, nil
}

// GetGoal returns a single goal. GET /api/goals/:id.
func (c *Client) GetGoal(goalID string) (*GoalRecord, error) {
	resp, err := c.http.Get(c.baseURL + "/api/goals/" + goalID)
	if err != nil {
		return nil, fmt.Errorf("client: get goal: %w", err)
	}
	defer resp.Body.Close()
	var goal GoalRecord
	if err := json.NewDecoder(resp.Body).Decode(&goal); err != nil {
		return nil, fmt.Errorf("client: decode goal: %w", err)
	}
	return &goal, nil
}

// PauseGoal pauses a goal. POST /api/goals/:id/pause.
func (c *Client) PauseGoal(goalID string) error {
	resp, err := c.http.Post(c.baseURL+"/api/goals/"+goalID+"/pause", "application/json", nil)
	if err != nil {
		return fmt.Errorf("client: pause goal: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("client: pause failed: %d", resp.StatusCode)
	}
	return nil
}

// ResumeGoal resumes a goal. POST /api/goals/:id/resume.
func (c *Client) ResumeGoal(goalID string) error {
	resp, err := c.http.Post(c.baseURL+"/api/goals/"+goalID+"/resume", "application/json", nil)
	if err != nil {
		return fmt.Errorf("client: resume goal: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("client: resume failed: %d", resp.StatusCode)
	}
	return nil
}

// StopGoal stops a goal. POST /api/goals/:id/stop.
func (c *Client) StopGoal(goalID string) error {
	resp, err := c.http.Post(c.baseURL+"/api/goals/"+goalID+"/stop", "application/json", nil)
	if err != nil {
		return fmt.Errorf("client: stop goal: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("client: stop failed: %d", resp.StatusCode)
	}
	return nil
}

// SystemStatus returns daemon system status. GET /api/system/status.
func (c *Client) SystemStatus() (map[string]interface{}, error) {
	resp, err := c.http.Get(c.baseURL + "/api/system/status")
	if err != nil {
		return nil, fmt.Errorf("client: system status: %w", err)
	}
	defer resp.Body.Close()
	var status map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("client: decode status: %w", err)
	}
	return status, nil
}

// GetGoalEvents 获取 Goal 的事件列表。GET /api/goals/:id/events（R-832 B11）。
func (c *Client) GetGoalEvents(goalID string) ([]map[string]interface{}, error) {
	resp, err := c.http.Get(c.baseURL + "/api/goals/" + goalID + "/events")
	if err != nil {
		return nil, fmt.Errorf("client: get events: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("client: get events failed: %d", resp.StatusCode)
	}
	var events []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		return nil, fmt.Errorf("client: parse events: %w", err)
	}
	return events, nil
}

// AdjustBudget 调整 Goal Token 预算。POST /api/goals/:id/budget（R-832 B11）。
func (c *Client) AdjustBudget(goalID, amount string) (map[string]interface{}, error) {
	body := map[string]string{"amount": amount}
	data, _ := json.Marshal(body)
	resp, err := c.http.Post(c.baseURL+"/api/goals/"+goalID+"/budget", "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("client: adjust budget: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("client: adjust budget failed: %d", resp.StatusCode)
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("client: parse budget response: %w", err)
	}
	return result, nil
}

// ListApprovals 待审批列表（GET /api/approvals——只读；写族=UDS-only）。
func (c *Client) ListApprovals() ([]map[string]interface{}, error) {
	resp, err := c.http.Get(c.baseURL + "/api/approvals")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var list []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("client: parse approvals: %w", err)
	}
	return list, nil
}

// postDecision 决策端点 POST（审批族——UDS-only；F-04 形状响应）。
func (c *Client) postDecision(actionID, action string) (map[string]interface{}, int, error) {
	resp, err := c.http.Post(c.baseURL+"/api/approvals/"+actionID+"/"+action, "application/json", nil)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&result)
	return result, resp.StatusCode, nil
}

// Approve 批准（POST /api/approvals/:id/approve——05 §2.2）。
func (c *Client) Approve(actionID string) (map[string]interface{}, int, error) {
	return c.postDecision(actionID, "approve")
}

// Reject 拒绝（POST /api/approvals/:id/reject）。
func (c *Client) Reject(actionID string) (map[string]interface{}, int, error) {
	return c.postDecision(actionID, "reject")
}

// WaitMore 审批延期（POST /api/approvals/:id/wait_more——D-4 R-1645）。
func (c *Client) WaitMore(actionID string) (map[string]interface{}, int, error) {
	return c.postDecision(actionID, "wait_more")
}

// InsertRequirement 需求注入（任务 5.8——04 权威形态 goalos insert <id> --requirement）。
// PUT /api/goals/{id}/requirements {text}（05 §2.2——200 {ok}）。
func (c *Client) InsertRequirement(goalID, text string) error {
	body := map[string]string{"text": text}
	data, _ := json.Marshal(body)
	req, err := http.NewRequest("PUT", c.baseURL+"/api/goals/"+goalID+"/requirements", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("需求注入失败: HTTP %d", resp.StatusCode)
	}
	return nil
}

// ─── ReviewReport API 方法（R-849 — 会议 #156）─────────────────────────

// GetReviews 获取 Goal 下所有审查摘要。
func (c *Client) GetReviews(goalID string) ([]map[string]interface{}, error) {
	resp, err := c.http.Get(c.baseURL + "/api/goals/" + goalID + "/reviews")
	if err != nil {
		return nil, fmt.Errorf("client: get reviews: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("client: get reviews failed: %d", resp.StatusCode)
	}
	var reviews []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&reviews); err != nil {
		return nil, fmt.Errorf("client: parse reviews: %w", err)
	}
	return reviews, nil
}

// GetReviewDetail 获取完整 ReviewReport（含所有 Provider 的 reasoning）。
func (c *Client) GetReviewDetail(goalID, actionID string) (map[string]interface{}, error) {
	resp, err := c.http.Get(c.baseURL + "/api/goals/" + goalID + "/reviews/" + actionID)
	if err != nil {
		return nil, fmt.Errorf("client: get review detail: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("client: get review detail failed: %d", resp.StatusCode)
	}
	var report map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		return nil, fmt.Errorf("client: parse review: %w", err)
	}
	return report, nil
}

// DecideReview 提交用户对 MultiLLM 审查结果的决策。
func (c *Client) DecideReview(goalID, actionID, decision, feedback string) (map[string]interface{}, error) {
	body := map[string]string{
		"decision": decision,
	}
	if feedback != "" {
		body["feedback"] = feedback
	}
	data, _ := json.Marshal(body)
	resp, err := c.http.Post(c.baseURL+"/api/goals/"+goalID+"/reviews/"+actionID+"/decide", "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("client: decide review: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("client: decide review failed: %d — %s", resp.StatusCode, string(bodyBytes))
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("client: parse decide response: %w", err)
	}
	return result, nil
}
