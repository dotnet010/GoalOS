// issuance.go——签发决策表实现（06 §1.3 签发决策表=R-1507 唯一权威；
// ExecutionContract v2 签发信息计算——任务 5.5 前置 daemon 生产接线，R-1640②）。
// 按序求值、首个匹配生效；T0 准入=静态判定（签发时——R-1528）。
package governance

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"os"
	"time"
)

// IssuanceInput 签发决策输入集（06 §1.3 四条件的事实源）。
type IssuanceInput struct {
	WorkloadRegistered    bool     // 名单登记命中且未过期（trusted_workloads——R-1549 运行时匹配产出）
	CapsSubsetDeclared    bool     // capability ⊆ 契约声明集（Capability Engine 评估产出）
	ArbitrarySubprocess   bool     // 含 shell.execute/生成代码（产生任意子进程）
	NetworkEgress         bool     // 涉网络出站（capability 含 web.*/browser.*/net.* 族）
	SensitivePathWrite    bool     // 涉敏感路径写入
	RiskLevel             string   // 风险级 R0-R5（Risk Engine 产出）
	PolicyExplicitI4      bool     // 治理策略显式声明 I4（policies.yaml——v0.3.1 无载体=false）
}

// IssuanceDecision 签发决策产出（ExecutionContract v2 两字段）。
type IssuanceDecision struct {
	RequiresRealEnforcement bool   // T0 适用性唯一判据
	MinIsolation            string // I 族 wire 值（I1/I2/I3/I4——I5=拒绝归解析层 R-1602）
}

// 任意子进程动作族（06 §1.3 行 2「shell.execute/生成代码」）。
var arbitrarySubprocessActions = map[string]bool{
	"shell.execute": true,
	"code.generate": true,
	"code.write":    true, // 生成代码族
}

// 网络出站能力前缀族（行 3「涉网络出站」）。
var networkEgressCaps = []string{"web.", "browser.", "net.", "http."}

// 敏感路径写入能力族（行 3「敏感路径写入」——目标路径判定归执行侧，签发侧按能力族）。
var sensitiveWriteCaps = []string{"fs.write", "fs.delete"}

// ComputeIssuanceDecision 签发决策表（按序求值首个匹配生效——06 §1.3）。
// 行 1：名单登记 ∧ caps⊆声明集 ∧ 无任意子进程 → {false, I1}
// 行 2：含 shell.execute/生成代码/未认证主体（名单外=未认证主体——兜底入本行） → {true, I2}
// 行 3：涉网络出站或敏感路径写入 → {true, I3}
// 行 4：风险级 R≥4 或治理策略显式声明 → {true, I4}
func ComputeIssuanceDecision(in IssuanceInput) IssuanceDecision {
	// 行 1（T0 准入——静态判定；RRE=false⇒MinIsolation≤I1 签发不变量 R-1601）
	if in.WorkloadRegistered && in.CapsSubsetDeclared && !in.ArbitrarySubprocess {
		return IssuanceDecision{RequiresRealEnforcement: false, MinIsolation: "I1"}
	}
	// 行 4 优先于行 2/3（风险级最高优先——表格按序，R≥4 显式最严）
	// 注：06 §1.3 表序=名单→shell/未认证→网络/敏感→R4；行 4 条件列「R≥4 或策略显式」
	// 独立成严级最高行——表序求值下 R≥4 若先命中行 2/3 会被低档位截获，
	// 与「R≥4=I4」语义冲突——按「风险级最高优先」语义修正求值序（RI-1 注记，会议 #257 登记）。
	if riskAtLeast(in.RiskLevel, "R4") || in.PolicyExplicitI4 {
		return IssuanceDecision{RequiresRealEnforcement: true, MinIsolation: "I4"}
	}
	// 行 3
	if in.NetworkEgress || in.SensitivePathWrite {
		return IssuanceDecision{RequiresRealEnforcement: true, MinIsolation: "I3"}
	}
	// 行 2（含兜底：名单外=未认证主体）
	return IssuanceDecision{RequiresRealEnforcement: true, MinIsolation: "I2"}
}

// riskAtLeast 风险级比较（R 族 wire 值——R-1114；兼容旧 L 族内部残留输入按等价序）。
func riskAtLeast(level, floor string) bool {
	ord := func(s string) int {
		switch s {
		case "R0", "L0":
			return 0
		case "R1", "L1":
			return 1
		case "R2", "L2":
			return 2
		case "R3", "L3":
			return 3
		case "R4", "L4":
			return 4
		case "R5", "L5":
			return 5
		}
		return -1 // 未知=最严档位之下（fail-closed：-1<floor 不触发提升）
	}
	return ord(level) >= ord(floor)
}

// ClassifyActionAttrs 动作属性归类（签发决策输入的事实源——capability/动作类型→三旗标）。
func ClassifyActionAttrs(actionType string, caps []string) (arbitrarySubprocess, networkEgress, sensitiveWrite bool) {
	arbitrarySubprocess = arbitrarySubprocessActions[actionType]
	for _, c := range caps {
		for _, p := range networkEgressCaps {
			if len(c) >= len(p) && c[:len(p)] == p {
				networkEgress = true
			}
		}
		for _, p := range sensitiveWriteCaps {
			if c == p {
				sensitiveWrite = true
			}
		}
	}
	return arbitrarySubprocess, networkEgress, sensitiveWrite
}

// ─── 名单登记匹配（签发侧——R-1508/R-1549④）───

// TrustedWorkloadView 名单条目最小视图（签发侧运行时匹配用——governance 不 import config，
// 分层纪律；publisher_key 不参与运行时验证——R-1561 厘清）。
type TrustedWorkloadView struct {
	ArtifactHash string // 64 字符小写 hex——工作负载主二进制 SHA-256（运行时验证值）
	ExpiresAt    string // ISO8601 可空（空=永不过期）
}

// MatchTrustedWorkload 名单匹配（artifact_hash 静态比对+未过期——R-1549④ 运行时匹配）：
// 命中未过期=true；缺失/过期/非法=false（过期=按行 2 重判，严禁静默留 T0）。
func MatchTrustedWorkload(list []TrustedWorkloadView, workloadHashHex string, now time.Time) bool {
	for _, w := range list {
		if w.ArtifactHash != workloadHashHex {
			continue
		}
		if w.ExpiresAt != "" {
			exp, err := time.Parse(time.RFC3339, w.ExpiresAt)
			if err != nil || now.After(exp) {
				continue // 已过期/非法（启动校验已拦非法——此处防御）=不匹配
			}
		}
		return true
	}
	return false
}

// ─── SessionID/Nonce 生成（05 §X.6.3——SessionID=唯一 ExecutionSession 绑定预分配；
// Nonce=32B crypto/rand hex，仅会话建立时单次消费——R-1510 时序句①）───

// newSessionID 会话 ID（crypto/rand 128bit——碰撞概率可忽略；前缀 sess- 审计可读）。
func newSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "sess-unknown" // crypto/rand 失败=签发侧拒绝在上层（Nonce 空=验证拒绝）
	}
	return fmt.Sprintf("sess-%x", b)
}

// newNonceHex 32B 随机数 hex（64 字符——验证层格式校验口径）。
func newNonceHex() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("issuance: nonce 生成失败: %w", err)
	}
	return fmt.Sprintf("%x", b), nil
}

// WorkloadHashOf 工作负载主二进制 SHA-256 hex（R-1561 运行时验证值计算点——
// 「工作负载二进制首次加载时计算 SHA-256」R-1549④）。
func WorkloadHashOf(binaryPath string) (string, error) {
	data, err := os.ReadFile(binaryPath)
	if err != nil {
		return "", fmt.Errorf("issuance: 工作负载二进制读取失败: %w", err)
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum), nil
}
