// Package client implements the GoalOS daemon HTTP client.
// Used by CLI and Web UI. All state is in the daemon — client is stateless.
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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
func New(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: defaultTimeout},
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
}

// GoalRecord is a single goal in list/get responses.
type GoalRecord struct {
	GoalID string `json:"goal_id"`
	Title  string `json:"title"`
	Status string `json:"status"`
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
