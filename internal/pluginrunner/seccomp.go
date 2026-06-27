// Package pluginrunner — seccomp 配置（v0.1.0 会议 #63 委托给 pkg/seccomp）。
// PluginRunner 使用 pkg/seccomp 生成 profile，通过 InitMessage 传给子进程自加载。
package pluginrunner

import "github.com/goalos/goalos/pkg/seccomp"

// SeccompProfile 类型别名——委托给 pkg/seccomp.Profile。
type SeccompProfile = seccomp.Profile

// DefaultSeccomp 返回 L0-L2 严格 seccomp 配置。
func DefaultSeccomp() *SeccompProfile { return seccomp.Default() }

// ExtendedSeccomp 返回 L3+ 扩展 seccomp 配置。
func ExtendedSeccomp() *SeccompProfile { return seccomp.Extended() }

// SeccompForRiskLevel 根据风险等级返回对应的 seccomp 配置。
func SeccompForRiskLevel(riskLevel string) *SeccompProfile { return seccomp.ForRiskLevel(riskLevel) }
