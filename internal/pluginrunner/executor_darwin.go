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

	"github.com/goalos/goalos/internal/sandbox"
)

// sanitizeChildProcess 在子进程启动前设置 macOS 安全加固。
// v0.3.0 fix (C6): 通过 sandbox-exec 实现文件系统/网络隔离。
// R-1641③（会议 #256）收敛：profile=internal/sandbox.RestrictedSeatbeltProfile() 单源
// （Option B 语义=写禁闭+敏感目录禁读+网络禁闭+子进程禁+读开放；E1 事故修复——
// 原内联 profile 含非法 filter 从未通过解析+读白名单形态启动期 abort，生产插件
// 沙箱路径自此真实生效）。若 sandbox-exec 不可用→降级为 Setpgid 基础隔离（L2 诚实标注）。
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
	// SBPL 按真实路径匹配——四处 -D 注入前统一 firmlink 规范化
	//（对齐 provider_darwin.go 生产面 convention；profile 契约=WORKSPACE_DIR/
	// TMP_DIR/HOME_DIR/TARGET_BINARY 四参——缺参=profile 编译失败 fail-closed）。
	for _, p := range []*string{&workspace, &tmpDir} {
		if c, err := filepath.EvalSymlinks(*p); err == nil {
			*p = c
		}
	}

	// 受限档 profile=单一来源（R-1641③——internal/sandbox embed；禁止内联副本）
	profile := sandbox.RestrictedSeatbeltProfile()

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
	if c, err := filepath.EvalSymlinks(homeDir); err == nil {
		homeDir = c
	}

	// 重写命令为 sandbox-exec（四参注入——profile 契约面，缺参=编译失败）
	origPath := cmd.Path
	if c, err := filepath.EvalSymlinks(origPath); err == nil {
		origPath = c
	}
	origArgs := cmd.Args
	cmd.Path = "/usr/bin/sandbox-exec"
	cmd.Args = append([]string{
		"sandbox-exec",
		"-f", profilePath,
		"-D", "WORKSPACE_DIR=" + workspace,
		"-D", "TMP_DIR=" + tmpDir,
		"-D", "HOME_DIR=" + homeDir,
		"-D", "TARGET_BINARY=" + origPath,
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

