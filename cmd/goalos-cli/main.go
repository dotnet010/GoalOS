// goalos CLI — GoalOS 命令行客户端。CLI 优先策略（R119）。
// 与 daemon 通过 HTTP 通信（localhost:18920）。
// 设计依据：04 CLI 设计规范 v6.1。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func printUsage() {
	fmt.Println(`GoalOS CLI — 面向人类目标的命令行接口

用法:
  goalos <目标描述>           创建并执行目标
  goalos new <描述>            创建新目标
  goalos list                  列出所有目标
  goalos status [goal_id]      查看目标详情或系统状态
  goalos pause <goal_id>       暂停目标
  goalos resume <goal_id>      恢复目标
  goalos stop <goal_id>        终止目标
  goalos health                系统状态（JSON）
  goalos help                  显示帮助`)
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
