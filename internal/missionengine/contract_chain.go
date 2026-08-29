// contract_chain.go——CompletionContract 版本链（R-1597/R-1613——任务 5.8 落地；
// 05 §6 合并规则唯一权威）：线性链——Revised=新版本，Superseded=旧版本终态；
// 继承锚点只增不减（缺任一已 Frozen 审批约束=契约违规 fail-closed）。
// 不做 DAG/三方合并（审计对象必须人脑可审——R-1597）。
package missionengine

import (
	"fmt"
	"sync"

	events "github.com/goalos/goalos/pkg/events"
)

// 版本链状态封闭枚举（R-1597）。
const (
	ContractActive     = "active"     // 首版/现行
	ContractRevised    = "revised"    // 修订后新版本（含新验收条款）
	ContractSuperseded = "superseded" // 被替代（终态——线性链旧版本）
)

// ContractVersion 完成契约单版本（领域对象——R-1613 存在论钉死：非 pkg/events struct）。
type ContractVersion struct {
	ContractID    string
	GoalID        string
	Version       int
	Status        string
	Supersedes    string // 被替代版本 ContractID（首版=空）
	FrozenAnchors []string
	Criteria      CompletionCriteria
}

// ContractChain 版本链管理器（per-Goal 线性链——追加式审计）。
type ContractChain struct {
	mu      sync.Mutex
	byGoal  map[string][]*ContractVersion // goalID → 版本链（version 升序）
	seq     int
}

// NewContractChain 构造空链管理器。
func NewContractChain() *ContractChain {
	return &ContractChain{byGoal: make(map[string][]*ContractVersion)}
}

// Record 首版登记（Goal 契约首次落地——version=1 status=active）。
func (c *ContractChain) Record(goalID string, criteria CompletionCriteria, anchors []string) *ContractVersion {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seq++
	v := &ContractVersion{
		ContractID:    fmt.Sprintf("%s-contract-%d", goalID, c.seq),
		GoalID:        goalID,
		Version:       1,
		Status:        ContractActive,
		FrozenAnchors: append([]string{}, anchors...),
		Criteria:      criteria,
	}
	c.byGoal[goalID] = []*ContractVersion{v}
	return v
}

// Revise 修订（RequirementAdded 消费链核心——R-1597 合并规则）：
// ①继承锚点只增不减：新版本 FrozenAnchors 必须 ⊇ 旧版本（缺任一=fail-closed 契约违规）；
// ②旧版本转 Superseded；③新版本 status=Revised、version+1、supersedes=旧 ContractID。
// 无既有版本=错误（修订必须先有首版——Record 先行）。
func (c *ContractChain) Revise(goalID string, criteria CompletionCriteria, newAnchors []string) (*ContractVersion, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	chain := c.byGoal[goalID]
	if len(chain) == 0 {
		return nil, fmt.Errorf("contract chain: goal %s 无首版契约——Record 先行（修订不能凭空）", goalID)
	}
	prev := chain[len(chain)-1]
	// ①继承锚点只增不减（fail-closed——R-1597：缺任一已审批约束=契约违规）
	for _, anchor := range prev.FrozenAnchors {
		found := false
		for _, na := range newAnchors {
			if na == anchor {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("contract chain: 继承锚点缺失 %q——已 Frozen 审批约束只增不减（fail-closed 契约违规）", anchor)
		}
	}
	// ②旧版本转 Superseded（终态——线性链）
	prev.Status = ContractSuperseded
	// ③新版本
	c.seq++
	nv := &ContractVersion{
		ContractID:    fmt.Sprintf("%s-contract-%d", goalID, c.seq),
		GoalID:        goalID,
		Version:       prev.Version + 1,
		Status:        ContractRevised,
		Supersedes:    prev.ContractID,
		FrozenAnchors: append([]string{}, newAnchors...),
		Criteria:      criteria,
	}
	c.byGoal[goalID] = append(chain, nv)
	return nv, nil
}

// Latest 现行版本（链尾——无=nil）。
func (c *ContractChain) Latest(goalID string) *ContractVersion {
	c.mu.Lock()
	defer c.mu.Unlock()
	chain := c.byGoal[goalID]
	if len(chain) == 0 {
		return nil
	}
	return chain[len(chain)-1]
}

// ChainLength 链长（审计/测试断言用）。
func (c *ContractChain) ChainLength(goalID string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.byGoal[goalID])
}

// toPayload 版本→事件载荷（07 §4 R-1414+R-1597 两字段+R-1613 载荷主体形态）。
func contractVersionPayload(v *ContractVersion) events.CompletionContractRecordedPayload {
	criteria := events.CompletionCriteria{
		GoalType:          v.Criteria.GoalType,
		SuccessDefinition: v.Criteria.SuccessDefinition,
	}
	return events.CompletionContractRecordedPayload{
		ContractID:    v.ContractID,
		Version:       v.Version,
		Status:        v.Status,
		Supersedes:    v.Supersedes,
		FrozenAnchors: append([]string{}, v.FrozenAnchors...),
		Criteria:      criteria,
	}
}
