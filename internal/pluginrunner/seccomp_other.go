//go:build !linux

package pluginrunner

import "fmt"

// ApplySeccomp 在非 Linux 平台上返回错误。
// macOS 使用 Seatbelt/sandbox-exec，Windows 使用 Job Objects——均需独立实现。
func ApplySeccomp(profile *SeccompProfile) error {
	return fmt.Errorf("seccomp: 当前平台不支持 seccomp（仅 Linux x86_64 支持）")
}
