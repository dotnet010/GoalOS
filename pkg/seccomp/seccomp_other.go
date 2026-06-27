//go:build !linux

// Package seccomp — 非 Linux 平台 stub。
package seccomp

// Apply 在非 Linux 平台上为 no-op（macOS 安全降级，R345）。
func Apply(profile *Profile) error {
	return nil
}
