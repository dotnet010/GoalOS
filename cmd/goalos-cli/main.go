// goalos CLI — GoalOS 命令行客户端。CLI 优先策略（R119）。
// 与 daemon 通过 HTTP 通信（localhost:18920）。
// 默认命令：goalos <目标描述> → 创建并等待 Goal 完成。
// 设计依据：04 CLI 设计规范 v6.1。
package main

import (
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
	if cmd != "daemon" {
		healthy, _ := c.Health()
		if !healthy {
			fmt.Println("Daemon 未运行。正在启动...")
			if err := startDaemon(); err != nil {
				fmt.Fprintf(os.Stderr, "无法启动 Daemon: %v\n", err)
				os.Exit(2)
			}
			// Wait for daemon to be ready
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
		goalText := os.Args[2]
		resp, err := c.CreateGoal(goalText)
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("目标已创建: %s\n", resp.GoalID)

	case "health", "status":
		healthy, err := c.Health()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Daemon 不可用: %v\n", err)
			os.Exit(1)
		}
		if healthy {
			fmt.Println("GoalOS Daemon: 运行中")
		}

	case "help", "--help", "-h":
		printUsage()

	default:
		// Default: treat as goal text
		goalText := cmd
		if len(os.Args) > 2 {
			goalText = os.Args[1] + " " + os.Args[2]
		}
		resp, err := c.CreateGoal(goalText)
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("目标已创建: %s\n", resp.GoalID)
		fmt.Println("状态: 进行中")
	}
}

func printUsage() {
	fmt.Println(`GoalOS CLI — 面向人类目标的命令行接口

用法:
  goalos <目标描述>          创建并执行目标（默认）
  goalos new <描述>           创建新目标
  goalos health               检查 Daemon 状态
  goalos help                 显示帮助`)
}

func startDaemon() error {
	// W2: auto-start daemon via os/exec.
	// 找到 goalos-daemon 二进制（与 CLI 同目录或 PATH 中）
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
	// 不等待——daemon 在后台运行
	return nil
}
