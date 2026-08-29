//go:build !darwin

// runtime_wiring_other.go——非 darwin 平台 Provider 注册占位（任务 5.1 Windows agentbox/
// 5.2 Linux seccomp+ns+cgroup 收敛前=注册表保持空——骨架纪律 R-1468 诚实状态，
// 非空壳：本函数存在性=平台分支的编译期完备性，非实现承诺）。
package main

// registerPlatformProvider 非 darwin=无注册（5.1/5.2 收敛后各平台文件落地）。
func registerPlatformProvider(_ *runtimeBoundary, _ string) {}
