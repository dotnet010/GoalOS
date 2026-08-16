// goalos CLI — GoalOS 命令行客户端。CLI 优先策略（R119）。
// 与 daemon 通过 HTTP 通信（localhost:18920）。
// 设计依据：04 CLI 设计规范 v6.1。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/goalos/goalos/internal/client"
)

const defaultDaemonURL = "http://localhost:18920"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	c := client.New(defaultDaemonURL)

	// Auto-start daemon if not running
	if cmd != "daemon" && cmd != "help" {
		if ok, _ := c.Health(); !ok {
			fmt.Println("Daemon 未运行。正在启动...")
			if err := startDaemon(); err != nil {
				fmt.Fprintf(os.Stderr, "错误: 无法启动 Daemon。请检查 ~/.goalos/logs/\n")
				os.Exit(2)
			}
			for i := 0; i < 50; i++ {
				time.Sleep(100 * time.Millisecond)
				if ok, _ := c.Health(); ok {
					break
				}
			}
		}
	}

	switch cmd {
	case "new":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "用法: goalos new <目标描述>")
			os.Exit(1)
		}
		resp, err := c.CreateGoal(os.Args[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("目标已创建: %s\n", resp.GoalID)
		// R-835: 轮询显示 LLM 执行进展
		if resp.Status == "created" {
			pollGoalProgress(c, resp.GoalID)
		}

	case "list":
		goals, err := c.ListGoals()
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		if len(goals) == 0 {
			fmt.Println("没有目标。使用 goalos new <描述> 创建。")
			return
		}
		fmt.Printf("%-12s %-20s %s\n", "ID", "标题", "状态")
		for _, g := range goals {
			fmt.Printf("%-12s %-20s %s\n", g.GoalID, truncate(g.Title, 20), g.Status)
		}

	case "status":
		if len(os.Args) < 3 {
			// Show system status
			status, err := c.SystemStatus()
			if err != nil {
				fmt.Fprintf(os.Stderr, "错误: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("GoalOS Daemon: 运行中\n  PID: %.0f  端口: %.0f  活跃 Goal: %.0f  运行时间: %v\n",
				status["pid"], status["port"], status["active_goals"], status["uptime"])
			return
		}
		goal, err := c.GetGoal(os.Args[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("%s: %s (%s)\n", goal.GoalID, goal.Title, goal.Status)

	case "pause":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "用法: goalos pause <goal_id>")
			os.Exit(1)
		}
		if err := c.PauseGoal(os.Args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("已暂停: %s\n", os.Args[2])

	case "resume":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "用法: goalos resume <goal_id>")
			os.Exit(1)
		}
		if err := c.ResumeGoal(os.Args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("已恢复: %s\n", os.Args[2])

	case "stop":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "用法: goalos stop <goal_id>")
			os.Exit(1)
		}
		if err := c.StopGoal(os.Args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("已终止: %s\n", os.Args[2])

	case "health":
		status, err := c.SystemStatus()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Daemon 不可用: %v\n", err)
			os.Exit(1)
		}
		data, _ := json.MarshalIndent(status, "", "  ")
		fmt.Println(string(data))

	case "events":
		// R-832 B11: goalos events <goal_id> [--format text]
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "用法: goalos events <goal_id> [--format text]")
			os.Exit(1)
		}
		goalID := os.Args[2]
		formatJSON := true
		if len(os.Args) > 3 && os.Args[3] == "--format" && len(os.Args) > 4 && os.Args[4] == "text" {
			formatJSON = false
		}
		events, err := c.GetGoalEvents(goalID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		if formatJSON {
			data, _ := json.MarshalIndent(events, "", "  ")
			fmt.Println(string(data))
		} else {
			for i, e := range events {
				fmt.Printf("%d. [%s] %s — %s\n", i+1,
					e["timestamp"], e["type"], e["goal_id"])
			}
		}

	case "adjust":
		// R-832 B11: goalos adjust --budget <goal_id> <amount>
		if len(os.Args) < 5 || os.Args[2] != "--budget" {
			fmt.Fprintln(os.Stderr, "用法: goalos adjust --budget <goal_id> <amount>")
			os.Exit(1)
		}
		resp, err := c.AdjustBudget(os.Args[3], os.Args[4])
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("预算已调整: %s → %s tokens\n", os.Args[3], resp["new_budget"])

	case "keepalive":
		handleKeepAlive(c, os.Args[2:])

	case "decision":
		handleDecision(c, os.Args[2:])

	case "confirm":
		handleConfirm(c, os.Args[2:])

	case "retry":
		handleRetry(c, os.Args[2:])

	case "requirements":
		handleRequirements(c, os.Args[2:])

	case "error":
		handleError(c, os.Args[2:])

	case "review":
		handleReview(c, os.Args[2:])

	case "help", "--help", "-h":
		printUsage()

	default:
		// Default: treat as goal text (blocking mode concept — W2)
		resp, err := c.CreateGoal(cmd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("目标已创建: %s\n状态: 进行中\n", resp.GoalID)
	}
}

// handleKeepAlive 处理 goalos keepalive 命令（任务 7.10——R-952 CLI 断线保活实现）。
// R-1474（发现 36——R-1472 范围修正）：keepalive=状态维持（非信息缺失）——骨架期伪装成功=
// 正确性风险（用户相信维持住了某个东西而它实际正在悄悄过期）——必须走 ErrNotImplemented。
func handleKeepAlive(c *client.Client, args []string) {
	// 骨架：断线保活实现归 7.10 完成态（Keep-Alive 心跳+重连后增量拉取）。
	fmt.Fprintln(os.Stderr, "错误: keepalive 命令骨架期未实现（R-1474——状态维持命令不适用 R-1472 过渡态 UX）")
	os.Exit(1)
}

// handleDecision 处理 goalos decision 命令（任务 7.20——R-1125 审批交互唯一呈现=CLI）。
// 子命令：list/approve/reject/wait_more——审批决策命令族。
func handleDecision(c *client.Client, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "用法: goalos decision list|approve|reject|wait_more <id>")
		os.Exit(1)
	}
	subCmd := args[0]
	switch subCmd {
	case "list":
		fmt.Println("[骨架] decision list——GET /api/approvals（daemon 端点既有）")
	case "approve":
		fmt.Println("[骨架] decision approve——POST /api/approvals/{id}/approve（daemon 端点既有）")
	case "reject":
		fmt.Println("[骨架] decision reject——POST /api/approvals/{id}/reject（daemon 端点既有）")
	case "wait_more":
		fmt.Println("[骨架] decision wait_more——POST /api/approvals/{id}/wait_more（daemon 端点既有）")
	default:
		fmt.Fprintf(os.Stderr, "未知子命令: %s\n", subCmd)
		os.Exit(1)
	}
}

// handleConfirm 处理 goalos confirm 命令（任务 7.20——前置状态 Planned/NeedsReview，R-1248）。
func handleConfirm(c *client.Client, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "用法: goalos confirm <goal_id>")
		os.Exit(1)
	}
	fmt.Println("[骨架] confirm——POST /api/goals/{id}/confirm（前置状态 Planned/NeedsReview）")
}

// handleRetry 处理 goalos retry 命令（任务 7.20——唯一恢复原语 R-1127）。
func handleRetry(c *client.Client, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "用法: goalos retry <goal_id> [--feedback \"...\"]")
		os.Exit(1)
	}
	fmt.Println("[骨架] retry——POST /api/goals/{id}/retry（唯一恢复原语 R-1127）")
}

// handleRequirements 处理 goalos requirements 命令（任务 7.20——需求注入）。
func handleRequirements(c *client.Client, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "用法: goalos requirements <goal_id> --add \"<需求>\"")
		os.Exit(1)
	}
	fmt.Println("[骨架] requirements——POST /api/goals/{id}/requirements（需求注入）")
}

// handleError 处理 goalos error 命令（R-1022——09 错误码知识库 v1）。
// 子命令：explain <CODE>——四段式错误码解释（用户可见消息+触发条件+用户行动指南）。
func handleError(c *client.Client, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "用法: goalos error explain <CODE>")
		fmt.Fprintln(os.Stderr, "  goalos error explain SBX-WIN-F-001   解释错误码含义与修复指引")
		os.Exit(1)
	}
	subCmd := args[0]
	switch subCmd {
	case "explain":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "用法: goalos error explain <CODE>")
			os.Exit(1)
		}
		code := args[1]
		// 骨架：错误码解释查询——实现归 daemon 端错误码知识库 API（09 §2 条目）。
		// 先红挂起：daemon 端点 /api/errors/{code} 未实现（W3-4 任务）。
		fmt.Printf("错误码: %s\n状态: [骨架] daemon 端错误码知识库 API 未实现（转绿归 W3-4）\n", code)
	default:
		fmt.Fprintf(os.Stderr, "未知子命令: %s\n用法: goalos error explain <CODE>\n", subCmd)
		os.Exit(1)
	}
}

// handleReview 处理 goalos review 命令（R-849 — 会议 #156）。
func handleReview(c *client.Client, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "用法: goalos review <goal_id> [<action_id>] [--retry|--accept|--refine]")
		fmt.Fprintln(os.Stderr, "  goalos review <goal_id>                    列出审查摘要")
		fmt.Fprintln(os.Stderr, "  goalos review <goal_id> <action_id>          查看完整审查报告")
		fmt.Fprintln(os.Stderr, "  goalos review <goal_id> <action_id> --retry [--feedback \"...\"]")
		fmt.Fprintln(os.Stderr, "  goalos review <goal_id> <action_id> --accept --confirm")
		fmt.Fprintln(os.Stderr, "  goalos review <goal_id> <action_id> --refine \"<需求描述>\"")
		os.Exit(1)
	}

	goalID := args[0]

	// 解析子命令
	if len(args) >= 3 {
		actionID := args[1]
		subCmd := args[2]

		switch subCmd {
		case "--retry":
			feedback := ""
			if len(args) >= 4 && args[3] == "--feedback" && len(args) >= 5 {
				feedback = args[4]
			}
			resp, err := c.DecideReview(goalID, actionID, "retry", feedback)
			if err != nil {
				fmt.Fprintf(os.Stderr, "错误: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("✅ %s\n", resp["message"])

		case "--accept":
			if len(args) < 4 || args[3] != "--confirm" {
				fmt.Println("⚠️  接受 AI 审查意见意味着你认可当前产出物，系统将继续执行。此操作会被审计记录。")
				fmt.Println("   请添加 --confirm 以确认接受风险。")
				os.Exit(1)
			}
			resp, err := c.DecideReview(goalID, actionID, "accept", "")
			if err != nil {
				fmt.Fprintf(os.Stderr, "错误: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("✅ %s\n", resp["message"])

		case "--refine":
			if len(args) < 4 {
				fmt.Fprintln(os.Stderr, "用法: goalos review <goal_id> <action_id> --refine \"<需求描述>\"")
				os.Exit(1)
			}
			newReq := args[3]
			resp, err := c.DecideReview(goalID, actionID, "refine", newReq)
			if err != nil {
				fmt.Fprintf(os.Stderr, "错误: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("✅ %s\n", resp["message"])

		default:
			fmt.Fprintf(os.Stderr, "未知子命令: %s\n", subCmd)
			os.Exit(1)
		}
		return
	}

	// goalos review <goal_id> [<action_id>] — 查看审查报告
	if len(args) >= 2 {
		actionID := args[1]
		report, err := c.GetReviewDetail(goalID, actionID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		printReviewDetail(report)
		return
	}

	// goalos review <goal_id> — 列出审查摘要
	reviews, err := c.GetReviews(goalID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
	if len(reviews) == 0 {
		fmt.Println("暂无 MultiLLM 审查记录。")
		return
	}
	fmt.Printf("%-12s %-8s %-30s %s\n", "ACTION ID", "VERDICT", "VOTE DISTRIBUTION", "TIME")
	fmt.Println(strings.Repeat("-", 80))
	for _, r := range reviews {
		icon := verdictIcon(r["verdict"].(string))
		dist := r["vote_distribution"].(map[string]interface{})
		fmt.Printf("%-12s %s %-8s %dF/%dW/%dP %s\n",
			r["action_id"], icon, r["verdict"],
			int(dist["fail"].(float64)), int(dist["warn"].(float64)),
			int(dist["pass"].(float64)),
			r["created_at"])
	}
}

// printReviewDetail 打印完整审查报告（Persona Level 0-1 渲染——事实+结构，不改变措辞）。
func printReviewDetail(report map[string]interface{}) {
	goalID := report["goal_id"]
	actionID := report["action_id"]
	verdict := report["verdict"].(string)
	dist := report["vote_distribution"].(map[string]interface{})

	fmt.Println(strings.Repeat("═", 60))
	fmt.Printf("MultiLLM 审查报告\n")
	fmt.Printf("Goal:   %s\n", goalID)
	fmt.Printf("Action: %s\n", actionID)
	fmt.Printf("裁决:   %s %s (%d FAIL / %d WARN / %d PASS)\n",
		verdict, verdictIcon(verdict),
		int(dist["fail"].(float64)), int(dist["warn"].(float64)),
		int(dist["pass"].(float64)))
	fmt.Println(strings.Repeat("─", 60))

	opinions, ok := report["provider_opinions"].([]interface{})
	if !ok {
		fmt.Println("(无 Provider 审查意见)")
		return
	}
	for _, o := range opinions {
		op := o.(map[string]interface{})
		vote := op["vote"].(string)
		fmt.Printf("[%s] %s — %s %s (%.0fms)\n",
			op["provider"], op["model"], vote, verdictIcon(vote),
			op["duration_ms"])
		fmt.Printf("  %s\n", op["reasoning"])
		fmt.Println()
	}
	fmt.Println(strings.Repeat("─", 60))

	// 用户决策状态
	ud, hasDecision := report["user_decision"].(map[string]interface{})
	if hasDecision && ud != nil {
		fmt.Printf("你的决定: %s (tainted=%v)\n", ud["decision"], ud["tainted"])
	} else {
		fmt.Println("你需要决定: [retry] [accept] [refine]")
	}
	fmt.Println(strings.Repeat("═", 60))
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// pollGoalProgress 轮询 Goal 状态并显示 LLM 进展（R-835）。
func pollGoalProgress(c *client.Client, goalID string) {
	lastStatus := ""
	for i := 0; i < 120; i++ {
		time.Sleep(2 * time.Second)
		goal, err := c.GetGoal(goalID)
		if err != nil {
			continue
		}
		if goal.Status != lastStatus {
			fmt.Printf("  → %s\n", statusText(goal.Status))
			lastStatus = goal.Status
		}
		// R-845: MultiLLM 审查结果实时展示
		if goal.MultiLLMVerdict != "" && goal.MultiLLMVerdict != lastVerdict {
			fmt.Printf("\n  ═══ MultiLLM 审查报告 ═══\n")
			fmt.Printf("  裁决: %s\n", verdictIcon(goal.MultiLLMVerdict)+" "+goal.MultiLLMVerdict)
			if goal.MultiLLMReport != "" {
				fmt.Printf("  %s\n", goal.MultiLLMReport)
			}
			fmt.Printf("  ─────────────────────────\n")
			fmt.Printf("  操作选项:\n")
			fmt.Printf("    goalos review %s --retry     → LLM 根据反馈重新生成\n", goalID)
			fmt.Printf("    goalos review %s --accept    → 接受当前代码\n", goalID)
			fmt.Printf("    goalos review %s --refine    → 修改需求描述\n", goalID)
			fmt.Printf("  ═══════════════════════════\n\n")
			lastVerdict = goal.MultiLLMVerdict
		}
		if goal.Status == "已完成" || goal.Status == "已失败" {
			return
		}
	}
	fmt.Println("  (等待超时——请用 goalos status " + goalID + " 查看)")
}

var lastVerdict string

func verdictIcon(v string) string {
	switch v {
	case "PASS":
		return "✅"
	case "WARN":
		return "⚠️"
	case "FAIL":
		return "❌"
	default:
		return ""
	}
}

func statusText(s string) string {
	switch s {
	case "created":
		return "目标已提交"
	case "正在分析目标":
		return "LLM 正在分析目标..."
	case "正在执行":
		return "正在执行任务..."
	case "已完成":
		return "✅ 目标完成"
	case "已失败":
		return "❌ 目标失败"
	default:
		return s
	}
}

func printUsage() {
	fmt.Println(`GoalOS CLI — 面向人类目标的命令行接口

用法:
  goalos <目标描述>                 创建并执行目标
  goalos new <描述>                  创建新目标
  goalos list                        列出所有目标
  goalos status [goal_id]            查看目标详情或系统状态
  goalos events <goal_id> [--format text] 查看目标事件链
  goalos adjust --budget <id> <amt>  调整目标 Token 预算
  goalos pause <goal_id>             暂停目标
  goalos resume <goal_id>            恢复目标
  goalos stop <goal_id>              终止目标
  goalos review <goal_id>             查看 MultiLLM 审查摘要
  goalos review <id> <action>         查看完整审查报告
  goalos review <id> <act> --retry    带反馈重试
  goalos review <id> <act> --accept --confirm  接受结果
  goalos review <id> <act> --refine "<desc>"   修改需求
  goalos health                      系统状态（JSON）
  goalos version                     显示版本
  goalos help                        显示帮助`)
}

func startDaemon() error {
	daemonPath := "goalos-daemon"
	if _, err := os.Stat("./goalos-daemon"); err == nil {
		daemonPath = "./goalos-daemon"
	}
	cmd := exec.Command(daemonPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 Daemon 失败: %w", err)
	}
	return nil
}
