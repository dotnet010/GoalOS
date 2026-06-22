//go:build !linux && !darwin

package pluginrunner

import (
	"fmt"
	"os/exec"
)

// ApplySeccomp 在非 Linux 平台上返回错误。
func ApplySeccomp(profile *SeccompProfile) error {
	return fmt.Errorf("seccomp: 当前平台不支持 seccomp（仅 Linux x86_64 支持）")
}

// applySeccompToChild 非 Linux/macOS 平台无 seccomp。
func applySeccompToChild(cmd *exec.Cmd, profile *SeccompProfile) {}
