//go:build !darwin && !linux && !windows

// runtime_wiring_other.go——其他平台无 Provider（注册表保持空——骨架纪律 R-1468 诚实状态，
// 非空壳：平台分支编译期完备性）。
package main

import "github.com/goalos/goalos/internal/llm"

func registerPlatformProvider(_ *runtimeBoundary, _ string, _ *llm.ZoneDialer) {}
