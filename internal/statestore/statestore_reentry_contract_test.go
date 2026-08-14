// 契约测试：Append 的快照回调不得在 s.mu 临界区内执行（R-1384）。
//
// 实测缺陷（P-7 顾问审查）：Append 持锁时调用 snapshotFn，而 daemon 注册的回调
// （cmd/goalos/main.go R-383 接线）会重入 store.SaveSnapshot——再次 s.mu.Lock()。
// Go sync.Mutex 不可重入：第 100 次 Append 触发回调即 runtime fatal deadlock。
//
// 隔离形态：本测试以子进程重跑自身（exec.Command re-exec，环境变量触发 helper 分支）
// 承载危险场景——死锁时 runtime 直接击杀 helper 进程（非 0 退出），不会拖死本测试包。
//
// 契约 MUST（R-1384）：
//   - MUST 1: Append 达到快照阈值（N=100）时触发回调且不产生自锁死锁（helper 必须 0 退出）。
//   - MUST 2: 回调（重入 SaveSnapshot）执行后，全部事件仍被持久化（101 条可回放）。
//   - MUST 3: 回调执行后快照文件确实落盘。
package statestore_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goalos/goalos/internal/statestore"
	"github.com/goalos/goalos/pkg/events"
)

// reentryHelperEnv 标记子进程进入 helper 分支（TestMain re-exec 模式）。
const reentryHelperEnv = "GOALOS_STATESTORE_REENTRY_HELPER"

// TestStoreWriterReentry_NoDeadlock 验证 Append 快照回调重入 Store 不死锁（R-1384）。
// 父进程：启动子进程执行危险场景（101 次 Append + main.go 同构重入回调）。
// 修复前：第 100 次 Append 锁内调用回调→回调 Lock 自锁→runtime deadlock→子进程非 0 退出。
// 修复后：回调移至锁外→子进程 0 退出，快照与事件均落盘。
func TestStoreWriterReentry_NoDeadlock(t *testing.T) {
	if os.Getenv(reentryHelperEnv) == "1" {
		runReentryHelper()
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestStoreWriterReentry_NoDeadlock$")
	cmd.Env = append(os.Environ(), reentryHelperEnv+"=1")
	out, err := cmd.CombinedOutput()

	// MUST 1: helper 必须 0 退出——自锁死锁会被 runtime 击杀（非 0）。
	if err != nil {
		t.Fatalf("R-1384 MUST 1 FAILED: reentry helper 非 0 退出（%v）——Append 持锁调用回调自锁死锁。output:\n%s",
			err, out)
	}

	// MUST 2: 事件持久化标记——回调重入未破坏事件追加（101 条全部落盘）。
	if !strings.Contains(string(out), "REENTRY_HELPER: events_appended=101") {
		t.Errorf("R-1384 MUST 2 FAILED: helper 未报告 101 条事件全部持久化。output:\n%s", out)
	}

	// MUST 3: 快照落盘标记——第 100 次 Append 触发的快照确实写入。
	if !strings.Contains(string(out), "REENTRY_HELPER: snapshot_written=true") {
		t.Errorf("R-1384 MUST 3 FAILED: helper 未报告快照落盘。output:\n%s", out)
	}
}

// runReentryHelper 在子进程中执行危险场景：注册重入回调（SaveSnapshot）→ 追加 101 事件。
// 任一契约断言失败→stderr 说明 + os.Exit(1)；全部通过→成功标记 + os.Exit(0)。
func runReentryHelper() {
	dir, err := os.MkdirTemp("", "goalos-reentry-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "helper: MkdirTemp: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(dir)

	store := statestore.New(dir)
	var snapErr error
	// 与 cmd/goalos/main.go R-383 接线同构：回调重入 store（LoadState + SaveSnapshot）。
	store.SetSnapshotCallback(func(goalID string) {
		state, err := store.LoadState(goalID)
		if err != nil {
			snapErr = fmt.Errorf("LoadState: %w", err)
			return
		}
		if state != nil {
			if err := store.SaveSnapshot(goalID, state); err != nil {
				snapErr = fmt.Errorf("SaveSnapshot: %w", err)
			}
		}
	})

	// 101 次 Append：第 100 次命中快照阈值（R-372 N=100）触发回调。
	for i := 1; i <= 101; i++ {
		if err := store.Append("goal_reentry", events.Event{
			Seq: i, Type: events.TypeGoalCreated, GoalID: "goal_reentry",
		}); err != nil {
			fmt.Fprintf(os.Stderr, "helper: Append #%d: %v\n", i, err)
			os.Exit(1)
		}
	}
	if snapErr != nil {
		fmt.Fprintf(os.Stderr, "helper: snapshot callback: %v\n", snapErr)
		os.Exit(1)
	}

	// MUST 2: 全部 101 个事件持久化（回调重入未破坏事件追加）。
	replayed, err := store.Replay("goal_reentry", 0)
	if err != nil || len(replayed) != 101 {
		fmt.Fprintf(os.Stderr, "helper: Replay: n=%d err=%v\n", len(replayed), err)
		os.Exit(1)
	}

	// MUST 3: 第 100 次 Append 触发的快照确已落盘。
	// state 未 SaveState→LoadState 返回 LastAppliedSeq=0→snapshot-0.json。
	if _, err := os.Stat(filepath.Join(dir, "goal_reentry", "snapshots", "snapshot-0.json")); err != nil {
		fmt.Fprintf(os.Stderr, "helper: snapshot 未落盘: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("REENTRY_HELPER: events_appended=101")
	fmt.Println("REENTRY_HELPER: snapshot_written=true")
	os.Exit(0)
}
