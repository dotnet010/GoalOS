// plugin-websearch — GoalOS Capability Plugin: Web Search (Bing API)
// stdin/stdout JSON 行协议（08沙箱规范 §3）。
// 在独立 OS 子进程中运行。调用 Bing Web Search API 并返回结构化结果。
package main

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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

const (
	serperEndpoint  = "https://google.serper.dev/search"
	bingEndpoint    = "https://api.bing.microsoft.com/v7.0/search"
	braveEndpoint   = "https://api.search.brave.com/res/v1/web/search"
)

var sessionToken string // R-660: HMAC signing token from InitMessage

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
			// R-660: 提取 session_token 用于 HMAC 签名
			var initMsg struct {
				SessionToken string `json:"session_token"`
			}
			if err := json.Unmarshal(line, &initMsg); err == nil && initMsg.SessionToken != "" {
				sessionToken = initMsg.SessionToken
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

	// 优先 Serper.dev，其次 Brave Search，最后 Bing
	if key := os.Getenv("SERPER_API_KEY"); key != "" {
		results, err := callSerperAPI(query, key)
		elapsed := int(time.Since(start).Milliseconds())
		if err != nil {
			writeResult(msg.ActionID, "failure", "", fmt.Sprintf("Serper: %v", err), elapsed)
			return
		}
		output, _ := json.MarshalIndent(results, "", "  ")
		writeResult(msg.ActionID, "success", string(output), "", elapsed)
		return
	}

	if key := os.Getenv("BRAVE_API_KEY"); key != "" {
		results, err := callBraveAPI(query, key)
		elapsed := int(time.Since(start).Milliseconds())
		if err != nil {
			writeResult(msg.ActionID, "failure", "", fmt.Sprintf("Brave API: %v", err), elapsed)
			return
		}
		output, _ := json.MarshalIndent(results, "", "  ")
		writeResult(msg.ActionID, "success", string(output), "", elapsed)
		return
	}

	if key := os.Getenv("BING_API_KEY"); key != "" {
		results, err := callBingAPI(query, key)
		elapsed := int(time.Since(start).Milliseconds())
		if err != nil {
			writeResult(msg.ActionID, "failure", "", fmt.Sprintf("Bing API: %v", err), elapsed)
			return
		}
		output, _ := json.MarshalIndent(results, "", "  ")
		writeResult(msg.ActionID, "success", string(output), "", elapsed)
		return
	}

	writeResult(msg.ActionID, "failure", "", "no search API key set (SERPER_API_KEY, BRAVE_API_KEY, or BING_API_KEY)", int(time.Since(start).Milliseconds()))
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

// ─── Serper.dev ───

type serperResponse struct {
	Organic []serperResult `json:"organic"`
}

type serperResult struct {
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	Link    string `json:"link"`
}

func callSerperAPI(query, apiKey string) (*searchOutput, error) {
	// R-660: 使用 json.Marshal 防 JSON 注入——非 fmt.Sprintf
	bodyBytes, _ := json.Marshal(map[string]interface{}{"q": query, "num": 10})
	body := string(bodyBytes)
	req, err := http.NewRequest("POST", serperEndpoint, strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create: %w", err)
	}
	req.Header.Set("X-API-KEY", apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Serper returned %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	var sResp serperResponse
	if err := json.NewDecoder(resp.Body).Decode(&sResp); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	output := &searchOutput{Query: query, Total: len(sResp.Organic)}
	for _, r := range sResp.Organic {
		output.Results = append(output.Results, searchItem{
			Title:   r.Title,
			Snippet: r.Snippet,
			URL:     r.Link,
		})
	}
	return output, nil
}

// ─── Brave Search API ───

type braveResponse struct {
	Web *braveWeb `json:"web"`
}

type braveWeb struct {
	Results []braveResult `json:"results"`
}

type braveResult struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
}

func callBraveAPI(query, apiKey string) (*searchOutput, error) {
	reqURL := fmt.Sprintf("%s?q=%s&count=10", braveEndpoint, url.QueryEscape(query))
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", apiKey)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Brave API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var braveResp braveResponse
	if err := json.NewDecoder(resp.Body).Decode(&braveResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	output := &searchOutput{Query: query}
	if braveResp.Web != nil {
		output.Total = len(braveResp.Web.Results)
		for _, r := range braveResp.Web.Results {
			output.Results = append(output.Results, searchItem{
				Title:   r.Title,
				Snippet: r.Description,
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

// writeJSON 使用 v0.2.0 两行 HMAC 协议：第一行 HMAC-SHA256 hex，第二行 JSON payload。
// R-660: 每条 IPC 消息必须有 HMAC 签名。
func writeJSON(v interface{}) {
	data, _ := json.Marshal(v)
	if sessionToken != "" {
		mac := hmac.New(sha256.New, []byte(sessionToken))
		mac.Write(data)
		sig := hex.EncodeToString(mac.Sum(nil))
		fmt.Println(sig)
	}
	fmt.Println(string(data))
}
