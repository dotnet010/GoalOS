// plugin-websearch — GoalOS Capability Plugin: Web Search (Bing API)
// stdin/stdout JSON 行协议（08沙箱规范 §3）。
// 在独立 OS 子进程中运行。调用 Bing Web Search API 并返回结构化结果。
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// ─── IPC 消息类型 (Daemon → Plugin, stdin) ───

type ipcMessage struct {
	Type         string                 `json:"type"`
	Config       map[string]interface{} `json:"config,omitempty"`
	Capabilities []string               `json:"capabilities,omitempty"`
	Workspace    string                 `json:"workspace,omitempty"`
	Tmp          string                 `json:"tmp,omitempty"`
	ActionID     string                 `json:"action_id,omitempty"`
	ActionType   string                 `json:"action_type,omitempty"`
	Target       string                 `json:"target,omitempty"`
	Params       map[string]interface{} `json:"params,omitempty"`
	TimeoutMs    int                    `json:"timeout_ms,omitempty"`
	Reason       string                 `json:"reason,omitempty"`
}

// ─── IPC 消息类型 (Plugin → Daemon, stdout) ───

type resultMessage struct {
	Type     string `json:"type"`
	ActionID string `json:"action_id"`
	Status   string `json:"status"`
	Output   string `json:"output,omitempty"`
	Error    string `json:"error,omitempty"`
	CostMs   int    `json:"cost_ms"`
}

type errorMessage struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ─── Bing API 响应结构 ───

type bingResponse struct {
	WebPages *bingWebPages `json:"webPages"`
}

type bingWebPages struct {
	Value []bingResult `json:"value"`
}

type bingResult struct {
	Name    string `json:"name"`
	Snippet string `json:"snippet"`
	URL     string `json:"url"`
}

// ─── 搜索结果输出结构 ───

type searchOutput struct {
	Query   string        `json:"query"`
	Results []searchItem  `json:"results"`
	Total   int           `json:"total_estimated"`
}

type searchItem struct {
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	URL     string `json:"url"`
}

const bingEndpoint = "https://api.bing.microsoft.com/v7.0/search"

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 65536), 65536)

	for scanner.Scan() {
		line := scanner.Bytes()
		var msg ipcMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			writeError("invalid_message", fmt.Sprintf("cannot parse: %v", err))
			continue
		}

		switch msg.Type {
		case "init":
			if msg.Workspace == "" {
				writeError("init_failed", "missing workspace path")
				os.Exit(1)
			}
		case "execute":
			handleExecute(msg)
		case "shutdown":
			os.Exit(0)
		default:
			writeError("invalid_message", fmt.Sprintf("unknown type: %s", msg.Type))
		}
	}

	if err := scanner.Err(); err != nil {
		writeError("internal_error", fmt.Sprintf("stdin error: %v", err))
		os.Exit(1)
	}
}

func handleExecute(msg ipcMessage) {
	start := time.Now()

	switch msg.ActionType {
	case "web.search":
		runBingSearch(msg, start)
	default:
		writeResult(msg.ActionID, "failure",
			"", fmt.Sprintf("unsupported action type: %s", msg.ActionType),
			int(time.Since(start).Milliseconds()))
	}
}

func runBingSearch(msg ipcMessage, start time.Time) {
	query := msg.Target
	if query == "" {
		if p, ok := msg.Params["query"]; ok {
			query = fmt.Sprint(p)
		}
	}
	if query == "" {
		writeResult(msg.ActionID, "failure", "", "no search query specified", int(time.Since(start).Milliseconds()))
		return
	}

	apiKey := os.Getenv("BING_API_KEY")
	if apiKey == "" {
		writeResult(msg.ActionID, "failure", "", "BING_API_KEY environment variable not set", int(time.Since(start).Milliseconds()))
		return
	}

	results, err := callBingAPI(query, apiKey)
	elapsed := int(time.Since(start).Milliseconds())
	if err != nil {
		writeResult(msg.ActionID, "failure", "", fmt.Sprintf("Bing API call failed: %v", err), elapsed)
		return
	}

	output, _ := json.MarshalIndent(results, "", "  ")
	writeResult(msg.ActionID, "success", string(output), "", elapsed)
}

func callBingAPI(query, apiKey string) (*searchOutput, error) {
	reqURL := fmt.Sprintf("%s?q=%s&count=10&mkt=zh-CN", bingEndpoint, url.QueryEscape(query))
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Ocp-Apim-Subscription-Key", apiKey)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Bing API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var bingResp bingResponse
	if err := json.NewDecoder(resp.Body).Decode(&bingResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	output := &searchOutput{
		Query: query,
	}
	if bingResp.WebPages != nil {
		output.Total = len(bingResp.WebPages.Value)
		for _, r := range bingResp.WebPages.Value {
			output.Results = append(output.Results, searchItem{
				Title:   r.Name,
				Snippet: r.Snippet,
				URL:     r.URL,
			})
		}
	}

	return output, nil
}

func writeResult(actionID, status, output, errStr string, costMs int) {
	writeJSON(resultMessage{
		Type:     "result",
		ActionID: actionID,
		Status:   status,
		Output:   output,
		Error:    errStr,
		CostMs:   costMs,
	})
}

func writeError(code, message string) {
	writeJSON(errorMessage{
		Type:    "error",
		Code:    code,
		Message: message,
	})
}

func writeJSON(v interface{}) {
	data, _ := json.Marshal(v)
	fmt.Println(string(data))
}
