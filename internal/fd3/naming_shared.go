//go:build windows || linux || darwin

// naming_shared.go——FD3 名段净化（定义面=使用面：仅 windows/linux 有传输实现——
// U1000 按构建上下文判定，2026-09-06 CI 实证事故纪律）。
package fd3

import "strings"

// sanitizeLabel 管道/socket 名段净化（字母数字横线族——熵化命名纪律的净化段，
// 与 winAC 同名纪律同构）。
func sanitizeLabel(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "x"
	}
	out := b.String()
	if len(out) > 24 {
		out = out[:24]
	}
	return out
}
