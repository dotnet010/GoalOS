// hash_chain_contract_test.go — WAL 哈希链契约测试（R-1393）。
//
// 断言来源: R-1393「prev_hash 补入信封权威表（第 13 字段；genesis=全零 32B）」——
// events.jsonl 逐行 SHA-256 链接，防线一/二/三语义挂接。
//
// 先红状态（当前为何红）: 当前 Append 原样编码 Event（无 prev_hash 键）——
// 第 2、3 行键缺失断言失败 → 测试红。
//
// 转绿任务: 3.19/3.17/1.39（WAL 行信封扩展 prev_hash 第 13 字段：genesis 行
// prev_hash=全零 32B hex；后续行 prev_hash=SHA-256(前一行原文)）。
package statestore_test

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goalos/goalos/internal/statestore"
	"github.com/goalos/goalos/pkg/events"
)

// TestWALHashChain_GenesisAndLink —— R-1393: 3 事件 WAL 必须是逐行哈希链——
// genesis 行 prev_hash=全零 32B hex，第 2、3 行 prev_hash=前一行的 SHA-256。
func TestWALHashChain_GenesisAndLink(t *testing.T) {
	dir := tempDir(t)
	store := statestore.New(dir)
	for i := 1; i <= 3; i++ {
		evt := events.NewEvent(events.TypeGoalCompleted, "goal_chain", "publisher")
		evt.Seq = i
		if err := store.Append("goal_chain", evt); err != nil {
			t.Fatalf("前置: Append #%d: %v", i, err)
		}
	}

	path := filepath.Join(dir, "goal_chain", "events.jsonl")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("前置: 打开 WAL: %v", err)
	}
	defer f.Close()
	var rawLines [][]byte
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := make([]byte, len(sc.Bytes()))
		copy(line, sc.Bytes())
		rawLines = append(rawLines, line)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("前置: 读 WAL: %v", err)
	}

	// MUST 1: 3 行 WAL。
	if len(rawLines) != 3 {
		t.Errorf("R-1393 MUST 1 FAIL: 期望 3 行 WAL，实际 %d 行", len(rawLines))
		return
	}
	var parsed []map[string]any
	var jsonParts [][]byte
	for _, line := range rawLines {
		// R-1453 envelope 格式：<json>\t<crc32_hex>\t<format_version>\n
		// 读取方契约：先按 \t 拆分取 JSON 部分，再 Unmarshal（prev_hash 语义不变）
		parts := bytes.SplitN(line, []byte{'\t'}, 3)
		jsonPart := parts[0]
		jsonParts = append(jsonParts, jsonPart)
		var m map[string]any
		if err := json.Unmarshal(jsonPart, &m); err != nil {
			t.Fatalf("前置: WAL 行 JSON 部分解析失败: %v", err)
		}
		parsed = append(parsed, m)
	}

	// MUST 2: genesis（第 1 行）prev_hash = 全零 32B hex（R-1393）。
	gh, ok := parsed[0]["prev_hash"]
	if !ok {
		t.Errorf("R-1393 MUST 2 FAIL: genesis 行缺 prev_hash 键（当前 WAL 行无第 13 字段）")
	} else if gh != strings.Repeat("0", 64) {
		t.Errorf("R-1393 MUST 2 FAIL: genesis prev_hash=%v，期望全零 32B hex", gh)
	}

	// MUST 3: 第 2、3 行含 prev_hash 键且值 = 前一行的 SHA-256（哈希链不断裂）。
	for i := 1; i < len(parsed); i++ {
		prev, ok := parsed[i]["prev_hash"]
		if !ok {
			t.Errorf("R-1393 MUST 3 FAIL: 第 %d 行缺 prev_hash 键——哈希链断裂", i+1)
			continue
		}
		sum := sha256.Sum256(jsonParts[i-1])
		if want := fmt.Sprintf("%x", sum); prev != want {
			t.Errorf("R-1393 MUST 3 FAIL: 第 %d 行 prev_hash=%v，期望 SHA-256(前一行)=%s", i+1, prev, want)
		}
	}
}
