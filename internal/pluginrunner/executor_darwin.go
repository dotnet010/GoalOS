//go:build darwin

// Package pluginrunner — macOS 子进程安全加固（v0.3.0）。
// sandbox-exec (Seatbelt) 集成实现文件系统和网络隔离。
// 设计依据：08 沙箱规范 §5.2、R-863 macOS L2 诚实标注。
package pluginrunner

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// sanitizeChildProcess 在子进程启动前设置 macOS 安全加固。
// v0.3.0 fix (C6): 通过 sandbox-exec 实现文件系统/网络隔离。
// sandbox-exec 默认拒绝文件系统和网络，仅允许 workspace + tmp + 系统库。
// 若 sandbox-exec 不可用→降级为 Setpgid 基础隔离（L2 诚实标注）。
func sanitizeChildProcess(cmd *exec.Cmd) {
	if applySandboxExec(cmd) {
		return
	}
	// sandbox-exec 不可用时的降级方案
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

// applySandboxExec 尝试使用 sandbox-exec 包裹子进程。
// 成功返回 true，不可用返回 false。
func applySandboxExec(cmd *exec.Cmd) bool {
	// 检查 sandbox-exec 是否可用
	if _, err := exec.LookPath("sandbox-exec"); err != nil {
		return false
	}

	workspace := os.Getenv("GOALOS_WORKSPACE")
	if workspace == "" {
		workspace = os.Getenv("HOME") + "/Goals"
	}
	tmpDir := os.Getenv("GOALOS_TMP")
	if tmpDir == "" {
		tmpDir = "/tmp/goalos"
	}

	// 动态生成 Seatbelt profile
	profile := `(version 1)
(deny default)
;; 允许读取工作区和临时目录
(allow file-read* file-write*
    (subpath "` + workspace + `")
    (subpath "` + tmpDir + `")
    (subpath "/usr/lib")
    (subpath "/System/Library")
    (literal "/dev/null")
    (literal "/dev/zero")
    (literal "/dev/random")
    (literal "/dev/urandom"))
;; 禁止访问 GoalOS 系统目录和敏感路径
(deny file-read* file-write*
    (subpath (string-append (param "HOME_DIR") "/.goalos"))
    (subpath (string-append (param "HOME_DIR") "/.ssh"))
    (subpath (string-append (param "HOME_DIR") "/.aws")))
;; 禁止创建子进程
(deny fork)
(deny process-exec)
(deny process-fork)
;; 禁止网络
(deny network*)
`

	// 写入临时 profile 文件
	profilePath := filepath.Join(tmpDir, "goalos-"+randomID()+".sb")
	if err := os.MkdirAll(filepath.Dir(profilePath), 0700); err != nil {
		return false
	}
	if err := os.WriteFile(profilePath, []byte(profile), 0600); err != nil {
		return false
	}

	// 获取 HOME 路径用于 Seatbelt 参数
	homeDir, _ := os.UserHomeDir()

	// 重写命令为 sandbox-exec
	origPath := cmd.Path
	origArgs := cmd.Args
	cmd.Path = "/usr/bin/sandbox-exec"
	cmd.Args = append([]string{
		"sandbox-exec",
		"-f", profilePath,
		"-D", "HOME_DIR=" + homeDir,
		"--",
		origPath,
	}, origArgs[1:]...)

	return true
}

func randomID() string {
	b := make([]byte, 8)
	for i := range b {
		b[i] = byte(syscall.Getpid()>>((i%4)*8)) ^ byte(i*37)
	}
	enc := "0123456789abcdef"
	out := make([]byte, 16)
	for i, v := range b {
		out[i*2] = enc[v>>4]
		out[i*2+1] = enc[v&0x0f]
	}
	return string(out)
}
