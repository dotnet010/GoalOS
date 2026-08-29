// builtin_profile.go——零配置内置基础 profile（D-1 A 方案——会议 #257 PM 裁决落地）。
// 规格缝合：05 §X.6.3 ProfileDigest=「生效 CompiledProfile 摘要」；R-1524 策略载体=
// 内置默认策略+policies.yaml 覆盖。本文件定义内置默认的确定性内容。
//
// 符号模板纪律（digest 确定性前提）：workspace/tmp/home 用标记（$WORKSPACE/$GOALOS_TMP/$HOME）——
// 执行侧路径替换发生在验证通过之后；digest 覆盖符号形态，签发/复核两侧可重算一致。
// v0.3.1 digest 范围注记（D-1 落地注记——会议 #257）：digest 覆盖=内置基础 profile
// （能力集/MinIsolation/平台派生）+PolicyRevision；三层合并的用户/项目层（~/.goalos/profiles、
// workspace/.goalos/profiles）在执行侧仍生效但不入 digest——层漂移检测=v0.4.0 评估（S 项登记）。
package sandbox

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
)

// 内置资源限额默认（builtin-v1 策略载体——R-1155 四键；收紧/放宽走 policies.yaml 覆盖）。
const (
	builtinMaxMemoryMB  = 512
	builtinMaxCPUCores  = 2
	builtinMaxDiskMB    = 1024
	builtinMaxProcesses = 1 // 默认无子进程（Option B 子进程禁——R-1641）
)

// BuiltinBaseProfile 零配置内置基础 profile（确定性派生——同输入集恒同输出）。
// 派生规则：
//
//	Isolation=入参 minIsolation（契约硬地板——R-1512③ 双路径关闭后唯一来源）
//	文件系统：写=工作区/临时目录（标记形态）；读=系统库/二进制面（平台分族）；
//	  敏感目录禁读（~/.goalos/.ssh/.aws/.config——R-1641 Option B 收口面）
//	网络：无网络能力=deny（socket 全拒——R-1156）；含网络能力=allowlist（能力族符号标记，
//	  实际端点清单=执行侧 plugin manifest network_allowlist 填充——验证后替换）
//	进程：含任意子进程能力=64；否则=1（无子进程）
func BuiltinBaseProfile(minIsolation string, caps []string, platform PlatformID) *RawProfile {
	sortedCaps := append([]string{}, caps...)
	sort.Strings(sortedCaps)

	net := NetworkSection{Mode: "deny"}
	var netCaps []string
	for _, c := range sortedCaps {
		for _, p := range []string{"web.", "browser.", "net.", "http."} {
			if strings.HasPrefix(c, p) {
				netCaps = append(netCaps, c)
			}
		}
	}
	if len(netCaps) > 0 {
		net = NetworkSection{Mode: "allowlist", Allowlist: netCaps} // 符号标记=能力族（执行侧填实际端点）
	}

	maxProc := builtinMaxProcesses
	for _, c := range sortedCaps {
		if c == "shell.execute" || c == "code.generate" || c == "code.write" {
			maxProc = 64 // 任意子进程能力族
			break
		}
	}

	return &RawProfile{
		Isolation: minIsolation,
		Filesystem: FilesystemSection{
			AllowRead:  builtinReadPaths(platform),
			AllowWrite: []string{"$WORKSPACE", "$GOALOS_TMP"},
			Deny: []string{
				"$HOME/.goalos", "$HOME/.ssh", "$HOME/.aws", "$HOME/.config",
			},
		},
		Network: net,
		Resources: ResourcesSection{
			MaxMemoryMB:  builtinMaxMemoryMB,
			MaxCPUCores:  builtinMaxCPUCores,
			MaxDiskMB:    builtinMaxDiskMB,
			MaxProcesses: maxProc,
		},
	}
}

// builtinReadPaths 平台分族读面（程序启动必需的系统库/二进制面——R-1641 E3 教训：
// 读白名单必须覆盖 dyld/加载器需求面，且按 firmlink 规范化后的真实路径书写）。
func builtinReadPaths(platform PlatformID) []string {
	switch platform {
	case "darwin":
		return []string{
			"$WORKSPACE", "$GOALOS_TMP",
			"/usr/lib", "/System/Library", "/System/Volumes/Preboot/Cryptexes",
			"/usr/bin", "/bin", "/sbin", "/usr/share",
		}
	case "windows":
		return []string{"$WORKSPACE", "$GOALOS_TMP", "C:\\Windows\\System32"}
	default: // linux/xinchuang 族
		return []string{
			"$WORKSPACE", "$GOALOS_TMP",
			"/usr/lib", "/lib", "/usr/bin", "/bin", "/sbin", "/usr/share", "/etc/ssl",
		}
	}
}

// BuiltinProfileDigest ProfileDigest 计算（05 §X.6.3——符号形态权威）：
// digest=hex(SHA-256(CanonicalKey(版本标记+平台+PolicyRevision+MinIsolation+能力集+profile 全字段线性化)))。
// 确定性契约：同输入集恒同 digest——签发侧（可缓存）与复核侧（永远独立重算——PM 裁决约束）一致。
func BuiltinProfileDigest(minIsolation string, caps []string, platform PlatformID, policyRevision string) (string, *RawProfile, error) {
	raw := BuiltinBaseProfile(minIsolation, caps, platform)
	sortedCaps := append([]string{}, caps...)
	sort.Strings(sortedCaps) // digest 独立于输入顺序
	if err := raw.Validate(); err != nil {
		return "", nil, fmt.Errorf("builtin profile 校验失败: %w", err)
	}
	fields := []string{
		"builtin-profile-v1", string(platform), policyRevision, minIsolation,
		strings.Join(sortedCaps, ","), // 能力集显式入摘要（契约绑定声明能力集——审计可读）
		// risk 不入摘要：其影响经 MinIsolation 传导（同档同 profile=同 digest——缓存复用正确性）
		strings.Join(raw.Filesystem.AllowRead, ","),
		strings.Join(raw.Filesystem.AllowWrite, ","),
		strings.Join(raw.Filesystem.Deny, ","),
		raw.Network.Mode, strings.Join(raw.Network.Allowlist, ","),
		fmt.Sprintf("%d/%d/%d/%d", raw.Resources.MaxMemoryMB, raw.Resources.MaxCPUCores,
			raw.Resources.MaxDiskMB, raw.Resources.MaxProcesses),
	}
	sum := sha256.Sum256([]byte(CanonicalKey(fields...)))
	return fmt.Sprintf("%x", sum), raw, nil
}
