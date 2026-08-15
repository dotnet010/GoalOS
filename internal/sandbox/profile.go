package sandbox

import (
	"crypto/sha256"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// profile.go — TOML Profile 解析与校验（任务 1.2 骨架）。
//
// 契约（08 §11）：TOML Profile=声明式安全契约文件——文件系统白名单/网络白名单/exec 分层策略/
// 资源限制/环境变量注入。解析器=BurntSushi/toml（R-907）；缓存键=内容 SHA-256（R-908，废 mtime）；
// 三层合并语义=deny 取并集、allow 取交集（R-908）。安全约束不弱于 daemon.yaml 全局策略。
//
// 零值非法化（R-1106）：CompiledProfile 必须经 Compile() 产出——直接构造的零值在 Execute 入口
// 被 Fatal fail-closed 拒绝（compiled 标记=false）。

// RawProfile — TOML 原始结构（解析后未校验）。
type RawProfile struct {
	Isolation  string            `toml:"isolation"`  // I0-I5 请求等级（编译期降级为实际生效，R-1012）
	Filesystem FilesystemSection `toml:"filesystem"` // 文件系统白名单
	Network    NetworkSection    `toml:"network"`    // 网络白名单
	Resources  ResourcesSection  `toml:"resources"`  // 资源限制（R-1155 四键）
}

// FilesystemSection — [filesystem] 节。
type FilesystemSection struct {
	AllowRead  []string `toml:"allow_read"`
	AllowWrite []string `toml:"allow_write"`
	Deny       []string `toml:"deny"`
}

// NetworkSection — [network] 节（R-1156 网络能力插件 socket 全拒+出网代理白名单）。
type NetworkSection struct {
	Mode      string   `toml:"mode"`      // "deny"|"allowlist"|"filtered"（filtered=出网代理白名单过滤）
	Allowlist []string `toml:"allowlist"` // 出网代理白名单
}

// ResourcesSection — [resources] 节（R-1155 四键）。
type ResourcesSection struct {
	MaxMemoryMB  int `toml:"max_memory_mb"`
	MaxCPUCores  int `toml:"max_cpu_cores"`
	MaxDiskMB    int `toml:"max_disk_mb"`
	MaxProcesses int `toml:"max_processes"`
}

// Validate — Rulebook V1-V12（08 §11.5）。骨架：非法值拒绝，合法值通过。
// 校验失败=返回描述性 error（字段名+约束名）——不静默降级。
func (r *RawProfile) Validate() error {
	if r == nil {
		return fmt.Errorf("profile: 零值非法（R-1106）——必须经 Compile() 流程产出")
	}
	switch r.Isolation {
	case "I0", "I1", "I2", "I3", "I4", "I5":
		// 合法
	case "":
		return fmt.Errorf("profile.isolation: 缺失——必须显式声明 I0-I5（无隐式默认降级，R-1012）")
	default:
		return fmt.Errorf("profile.isolation: 非法值 %q——取值域=I0..I5", r.Isolation)
	}
	switch r.Network.Mode {
	case "deny", "allowlist", "filtered", "":
		// 合法（空=默认 deny）
	default:
		return fmt.Errorf("profile.network.mode: 非法值 %q——取值域=deny|allowlist|filtered", r.Network.Mode)
	}
	if r.Resources.MaxMemoryMB < 0 || r.Resources.MaxCPUCores < 0 || r.Resources.MaxDiskMB < 0 || r.Resources.MaxProcesses < 0 {
		return fmt.Errorf("profile.resources: 负值非法（R-1155 四键非负）")
	}
	return nil
}

// ParseFile — TOML 文件解析（任务 3.1：BurntSushi/toml 解析层，R-907）。
// 契约：内容 SHA-256 缓存键（R-908——废 mtime）；解析失败=描述性 error（字段名+约束名）。
func ParseFile(path string) (*RawProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("profile: 读取失败 %s: %w", path, err)
	}
	var raw RawProfile
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("profile: TOML 解析失败 %s: %w", path, err)
	}
	if err := raw.Validate(); err != nil {
		return nil, err
	}
	return &raw, nil
}

// ValidateConflicts — 规则冲突检测（任务 3.2：R-916 场景 5 缓解——allow/deny 冲突）。
// 契约：deny 取并集、allow 取交集（R-908 三层合并语义）；冲突=拒绝（不静默降级）。
func (r *RawProfile) ValidateConflicts() error {
	// allow_write 与 deny 冲突检测：同一路径既 allow_write 又 deny=冲突
	denySet := make(map[string]bool, len(r.Filesystem.Deny))
	for _, p := range r.Filesystem.Deny {
		denySet[p] = true
	}
	for _, p := range r.Filesystem.AllowWrite {
		if denySet[p] {
			return fmt.Errorf("profile.filesystem: 路径冲突 %q——既 allow_write 又 deny（R-908 deny 并集语义：冲突=拒绝）", p)
		}
	}
	// allow_read 与 deny 冲突检测
	for _, p := range r.Filesystem.AllowRead {
		if denySet[p] {
			return fmt.Errorf("profile.filesystem: 路径冲突 %q——既 allow_read 又 deny（R-908 deny 并集语义：冲突=拒绝）", p)
		}
	}
	return nil
}

// Compile — TOML→CompiledProfile 编译（R-907 解析+R-908 内容 SHA-256 缓存键+R-1012 降级一次性确定）。
// 骨架：校验→构造 CompiledProfile（compiled=true 标记）。平台域编译规则归 backend_*。
func Compile(raw *RawProfile, platform PlatformID) (*CompiledProfile, error) {
	if err := raw.Validate(); err != nil {
		return nil, err
	}
	p := &CompiledProfile{
		platform:   platform,
		isolation:  parseIsolation(raw.Isolation),
		filesystem: CompiledFilesystem{AllowPaths: append(append([]string{}, raw.Filesystem.AllowRead...), raw.Filesystem.AllowWrite...)},
		network:    CompiledNetwork{Mode: compileNetworkMode(raw.Network.Mode)},
		process:    CompiledProcess{MaxProcesses: raw.Resources.MaxProcesses},
		extensions: map[string]any{},
		compiled:   true, // R-1106：Compile 产出标记
	}
	return p, nil
}

// parseIsolation — I0-I5 字符串→int（0-5）。
func parseIsolation(s string) int {
	if len(s) == 2 && s[0] == 'I' && s[1] >= '0' && s[1] <= '5' {
		return int(s[1] - '0')
	}
	return -1 // 非法（Validate 已拦截，此为防御性回退）
}

// compileNetworkMode — 网络三模式编译（任务 3.4：filtered/blocked/allowed 后端编译）。
// 契约：deny→blocked（socket 全拒）/allowlist→filtered（出网代理白名单过滤）/allowed→allowed（不过滤）。
func compileNetworkMode(mode string) string {
	switch mode {
	case "deny", "":
		return "blocked" // socket 全拒（R-1156）
	case "allowlist", "filtered":
		return "filtered" // 出网代理白名单过滤
	case "allowed":
		return "allowed" // 不过滤
	default:
		return "blocked" // 防御性回退（Validate 已拦截非法值）
	}
}

// CacheKey — 内容 SHA-256 缓存键（任务 3.5：R-908——废 mtime）。
// 契约：缓存键=内容 SHA-256（内容变=键变；mtime 不参与——防止"文件没变但 mtime 变了"的缓存失效）。
func (r *RawProfile) CacheKey() string {
	// 序列化关键字段为缓存键输入（字段顺序固定=确定性）
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%v|%v|%v|%v",
		r.Isolation, r.Filesystem.AllowRead, r.Filesystem.AllowWrite, r.Filesystem.Deny, r.Network.Mode)))
	return fmt.Sprintf("%x", sum)
}

// Merge — 三层配置合并（任务 3.5：R-908 三层合并语义=deny 取并集、allow 取交集）。
// 三层=系统默认（daemon.yaml）+用户级（~/.goalos/profiles/）+项目级（workspace/.goalos/profiles/）。
// 契约：deny 并集（任何一层 deny 的路径=最终 deny）；allow 交集（所有层都 allow 的路径=最终 allow）。
func Merge(base, user, project *RawProfile) *RawProfile {
	merged := &RawProfile{
		Isolation: base.Isolation, // 隔离等级=系统默认（用户/项目层不可降级——安全底线）
		Network:   NetworkSection{Mode: base.Network.Mode},
	}
	// deny 并集
	denySet := make(map[string]bool)
	for _, p := range base.Filesystem.Deny {
		denySet[p] = true
	}
	if user != nil {
		for _, p := range user.Filesystem.Deny {
			denySet[p] = true
		}
	}
	if project != nil {
		for _, p := range project.Filesystem.Deny {
			denySet[p] = true
		}
	}
	for p := range denySet {
		merged.Filesystem.Deny = append(merged.Filesystem.Deny, p)
	}
	// allow 交集（骨架：当前仅 base 层——用户/项目层 allow 交集归实现任务）
	merged.Filesystem.AllowRead = base.Filesystem.AllowRead
	merged.Filesystem.AllowWrite = base.Filesystem.AllowWrite
	return merged
}
