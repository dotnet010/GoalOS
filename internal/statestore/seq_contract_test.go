// seq_contract_test.go — 全局 seq 唯一分配者契约测试（R-1373）。
//
// 断言来源: R-1373「全局 seq 分配器唯一权威=WAL 写入器」——Append 时由 WAL
// 追加路径统一分配全局单调递增 seq，发布方（各模块）不得自设 seq。
//
// 先红状态（当前为何红）: 当前 Append 原样编码调用方事件（Seq 默认 0）——
// 同一 Goal 追加的 10 个事件全部 seq=0 → 唯一性断言失败 → 测试红。
//
// 转绿任务: 3.19/3.17（WAL 写入器在 Append 路径分配 global_seq，statestore
// 为唯一分配点；per-Goal seq 保留为聚合体内排序次键）。
package statestore_test

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"


	"github.com/goalos/goalos/internal/statestore"
	"github.com/goalos/goalos/pkg/events"
)

// TestGlobalSeq_UniqueAcrossPublishers —— R-1373: 同一 Goal 下多个发布方追加
// 事件（均不设置 Seq）必须被 WAL 写入器分配互不相等、全局单调递增的 seq。
func TestGlobalSeq_UniqueAcrossPublishers(t *testing.T) {
	dir := tempDir(t)
	store := statestore.New(dir)
	const n = 10

	// MUST 1 前置: 发布方不设置 Seq——seq 必须由 WAL 写入器分配（R-1373）。
	for i := 0; i < n; i++ {
		evt := events.NewEvent(events.TypeGoalCompleted, "goal_seq", "publisher")
		if err := store.Append("goal_seq", evt); err != nil {
			t.Fatalf("前置: Append #%d: %v", i, err)
		}
	}

	// 直接读 WAL 文件：Replay(0) 按 seq>0 过滤，seq 全 0 时返回空集无法诊断。
	path := filepath.Join(dir, "goal_seq", "events.jsonl")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("前置: 打开 WAL: %v", err)
	}
	defer f.Close()
	var seqs []int
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var evt struct {
			Seq int `json:"seq"`
		}
		jsonPart, _ := parseWALLine(sc.Bytes())
		if err := json.Unmarshal(jsonPart, &evt); err != nil {
			t.Fatalf("前置: 解析 WAL 行: %v", err)
		}
		seqs = append(seqs, evt.Seq)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("前置: 读 WAL: %v", err)
	}

	// MUST 1: 全部事件落盘。
	if len(seqs) != n {
		t.Errorf("R-1373 MUST 1 FAIL: 期望 %d 条事件，实际 %d 条", n, len(seqs))
	}
	// MUST 2: seq 全局唯一——WAL 写入器唯一分配，不得出现重复。
	seen := make(map[int]bool, len(seqs))
	for i, s := range seqs {
		if seen[s] {
			t.Errorf("R-1373 MUST 2 FAIL: seq=%d 重复（事件 #%d）——WAL 写入器未分配全局唯一 seq。seqs=%v", s, i, seqs)
		}
		seen[s] = true
	}
	// MUST 3: seq 全局单调递增（1..n）——分配器接管发布方。
	for i, s := range seqs {
		if s != i+1 {
			t.Errorf("R-1373 MUST 3 FAIL: 期望单调递增 seq=%d，实际 %d——全局分配器未接管 Append。seqs=%v", i+1, s, seqs)
		}
	}
}
