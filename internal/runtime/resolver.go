// resolver.go——Runtime Resolver 决策表全表（05 §X.6.4 权威；解析期——每个
// ExecutionSession 解析一次并冻结，Execute 路径零判断，R-1012；任务 3.1 全表施工）。
// 行 1（T0 名单登记——R-1601 硬地板显式条件 MinIsolation≤I1）/行 2（名单缺失/过期→
// 按 true 重判，严禁静默留 T0——R-1549-4）/行 3（RRE=true ∧ MinIsolation≤I3 ∧ 平台达成
// ≥MinIsolation 且已验证——R-1544）/行 3b（平台落差→治理升级标记，非静默降级——R-1544
// 平台落差治理）/行 4（MinIsolation=I4 ∧ 本机 I4 后端可用→T2——v0.3.1 无后端=永不命中，
// R-1599 诚实降级）/行 5（I4 需求+后端不可用→拒绝，T3 分支 v0.3.1=拒绝 R-1483）/
// 行 6（无候选拒绝；I5=i5_not_implemented——R-1602）。
package runtime

import (
	"fmt"
	"time"
)

// TrustedWorkload trusted_workloads 名单条目（05 §X.6.3 R-1508/R-1549 最小规范）。
type TrustedWorkload struct {
	PublisherKey string `yaml:"publisher_key"` // 64 字符小写 hex——身份标签/审计分组（v0.3.1 不验签，R-1561/R-1589）
	ArtifactHash string `yaml:"artifact_hash"` // 64 字符小写 hex——运行时验证值（SHA-256 静态比对）
	ExpiresAt    string `yaml:"expires_at"`    // ISO8601 可空（空=永不过期）
}

// ResolveInput Resolver 输入（解析期一次冻结的输入集）。
type ResolveInput struct {
	RequiresRealEnforcement bool           // 签发决策表产出（06 §1.3）——本层不重算（R-1507）
	MinIsolation            IsolationLevel // 契约硬地板（typed I 族——R-1628-3）
	WorkloadHashHex         string         // 工作负载主二进制 SHA-256 hex（运行时验证值——R-1561）
}

// Selection 解析结果（冻结——RuntimeSelected 事件数据源）。
type Selection struct {
	Tier                       ExecutionTier // 命中档位（T0/T1/T2——T3 v0.3.1 不做）
	Reason                     string        // 决策表命中行标识（机器值——selection_reason）
	MatchedWorkload            string        // 行 1 命中=名单条目 publisher_key 短码前 8 字符；非名单路径=省略（R-1589）
	NeedsGovernanceEscalation  bool          // 行 3b 命中=平台落差治理升级（人工审批显式降级——非静默降级）
	PlatformGap                string        // 行 3b 命中=平台落差描述（诚实呈现数据源）
}

// Resolver 解析器（决策表按序求值首个匹配生效；签发字段不重算）。
type Resolver struct {
	trusted       []TrustedWorkload
	now           func() time.Time                  // 时钟注入（测试可控）
	platformMax   func() IsolationLevel             // 平台实际达成 I 级探测（生产=internal/sandbox detect）
	i4Available   func() bool                       // 本机 I4 后端可用探测（v0.3.1=永不命中——R-1599；默认 false）
	onEvent       func(eventType string, sel Selection, rejectDetail string) // 事件发射点（nil=不发射——daemon 生产接线=W5 任务 5.5 前置，R-1640-2）
}

// NewResolver 构造解析器（名单=启动校验四规则已过的载入结果——config 层 R-1549）。
// platformMax 未注入时默认=I2（单面强制——保守下限，Darwin Seatbelt 唯一值）；
// 生产接线=任务 5.x 探测收敛。
func NewResolver(trusted []TrustedWorkload) *Resolver {
	return &Resolver{
		trusted:     trusted,
		now:         time.Now,
		platformMax: func() IsolationLevel { return I2 },
		i4Available: func() bool { return false },
	}
}

// WithClock 注入时钟（测试用——名单过期边界断言）。
func (r *Resolver) WithClock(now func() time.Time) *Resolver { r.now = now; return r }

// WithPlatformMaxIsolation 注入平台达成探测（行 3/3b 判定依据）。
func (r *Resolver) WithPlatformMaxIsolation(f func() IsolationLevel) *Resolver {
	r.platformMax = f
	return r
}

// WithI4BackendAvailable 注入 I4 后端可用探测（行 4——v0.3.1 恒 false，R-1599）。
func (r *Resolver) WithI4BackendAvailable(f func() bool) *Resolver { r.i4Available = f; return r }

// WithEventHook 注入事件发射点（RuntimeSelected/RuntimeSelectionRejected——任务 1.5 接线）。
func (r *Resolver) WithEventHook(f func(eventType string, sel Selection, rejectDetail string)) *Resolver {
	r.onEvent = f
	return r
}

// matchWorkload 运行时匹配（R-1549-4）：artifact_hash 静态比对+未过期；
// 命中返回条目短码（publisher_key 前 8 字符——R-1589 身份标签读取点）。
// 已过期条目=不匹配（走行 2 重判——严禁静默留 T0）。
func (r *Resolver) matchWorkload(hashHex string, now time.Time) (shortCode string, ok bool) {
	for _, w := range r.trusted {
		if w.ArtifactHash != hashHex {
			continue
		}
		if w.ExpiresAt != "" {
			exp, err := time.Parse(time.RFC3339, w.ExpiresAt)
			if err != nil || now.After(exp) {
				continue // 已过期（或非法——启动校验已拦非法，此处防御）=不匹配
			}
		}
		if len(w.PublisherKey) >= 8 {
			return w.PublisherKey[:8], true
		}
		return w.PublisherKey, true
	}
	return "", false
}

// Resolve 解析（决策表按序求值首个匹配生效——05 §X.6.4 全表）。
func (r *Resolver) Resolve(in ResolveInput) (Selection, error) {
	// 行 1（R-1601 硬地板显式条件）：RRE=false ∧ MinIsolation≤I1 ∧ 名单登记命中未过期 → T0
	if !in.RequiresRealEnforcement && in.MinIsolation <= I1 && in.MinIsolation >= I0 {
		if short, ok := r.matchWorkload(in.WorkloadHashHex, r.now()); ok {
			sel := Selection{Tier: TierT0, Reason: "row1_trusted_workload", MatchedWorkload: short}
			r.emit("RuntimeSelected", sel, "")
			return sel, nil
		}
	}
	// 行 2：RRE=false 但名单登记缺失/未命中/已过期 → 按 true 重新判定（不静默留 T0——R-1549-4）
	rre := in.RequiresRealEnforcement
	row2Hit := false
	if !rre {
		rre = true // 行 2 重判
		row2Hit = true
	}
	// 行 3：RRE=true ∧ MinIsolation≤I3 ∧ 平台达成≥MinIsolation → T1 受限档
	if rre && in.MinIsolation <= I3 {
		platformMax := r.platformMax()
		if platformMax >= in.MinIsolation {
			reason := "row3_restricted"
			if row2Hit {
				// 行 2 命中事实保留（R-1550——依据行警告样式呈现的前提前提：
				// 名单登记缺失/已过期=配置异常状态，不以正常决策行伪装——04 §14.2）
				reason = "row2_restricted"
			}
			sel := Selection{Tier: TierRestricted, Reason: reason}
			r.emit("RuntimeSelected", sel, "")
			return sel, nil
		}
		// 行 3b：平台落差 → 治理升级（人工审批显式降级——用户知情同意，非静默降级）
		sel := Selection{
			Tier:                      TierRestricted,
			Reason:                    "row3b_platform_gap",
			NeedsGovernanceEscalation: true,
			PlatformGap:               fmt.Sprintf("平台达成 %s < 契约要求 %s", platformMax, in.MinIsolation),
		}
		r.emit("RuntimeSelected", sel, "")
		return sel, nil
	}
	// 行 4/5：I4 需求
	if rre && in.MinIsolation == I4 {
		if r.i4Available() {
			// 行 4：本机 I4 后端可用 → T2（v0.3.1 永不命中——R-1599；分支保留=逻辑正确性锚点）
			sel := Selection{Tier: TierHardenedLocal, Reason: "row4_hardened_local"}
			r.emit("RuntimeSelected", sel, "")
			return sel, nil
		}
		// 行 5：后端不可用 → 拒绝（T3 分支 v0.3.1=拒绝——R-1483）
		err := fmt.Errorf("runtime selection rejected: MinIsolation=I4 且本机 I4 后端不可用（T3 分支 v0.3.1=拒绝——R-1483/R-1599）")
		r.emit("RuntimeSelectionRejected", Selection{}, "i4_backend_unavailable")
		return Selection{}, err
	}
	// 行 6：无满足候选=拒绝（严禁降档）；I5 显式=i5_not_implemented（R-1602）
	if in.MinIsolation == I5 {
		err := fmt.Errorf("runtime selection rejected: i5_not_implemented（I5 实现推迟 v0.4.0——R-1602/C-PLAT-10）")
		r.emit("RuntimeSelectionRejected", Selection{}, "i5_not_implemented")
		return Selection{}, err
	}
	err := fmt.Errorf("runtime selection rejected: 无满足候选（严禁降级到更弱档——05 §X.6.4 行 6）")
	r.emit("RuntimeSelectionRejected", Selection{}, "no_candidate")
	return Selection{}, err
}

// emit 事件发射（nil hook=不发射——daemon 生产接线=W5 任务 5.5 前置，R-1640-2）。
func (r *Resolver) emit(eventType string, sel Selection, rejectDetail string) {
	if r.onEvent != nil {
		r.onEvent(eventType, sel, rejectDetail)
	}
}
