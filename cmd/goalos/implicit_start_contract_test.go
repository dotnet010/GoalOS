// 隐式启动并发互斥契约（R-1326 / R-1394 / R-1378）。
//
// 契约（resolutions.yaml R-1326 修订注记）:
//   并发互斥 = O_EXCL pidfile（非 flock）。
//   判定指纹: 并发两次 acquirePIDLock → 恰一个成功；败者错误必须为 EEXIST 族
//   （errors.Is(err, fs.ErrExist)）。flock 语义下双方 open 均成功——
//   EEXIST 是 O_EXCL 独有的互斥指纹。
//
// 先红状态: 当前 main.go acquirePIDLock 已用 O_EXCL 实现 → 本测试预期绿
// （阶段 3.5 允许保留绿断言——作为回归护栏，防止实现退化为 flock/普通锁文件）。
// 红锚: 若实现退化为 flock 或非原子锁文件，并发唯一性与 EEXIST 指纹同时失效 → 红。
//
// 转绿任务: 1.19（C-2 表）——本测试为隐式启动互斥的回归护栏。
//
// 断言方式: 行为断言（并发调用 + 错误类型判定）——禁止读源码文本断言。
// 注意: 注释性断言"flock 字样不出现"不可行（无法对注释做行为断言），
// 以函数存在性（acquirePIDLock）+ 并发唯一性 + EEXIST 指纹三者代替。
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// TestImplicitStart_ConcurrentSpawn — 隐式启动并发互斥。
//
// 两次并发隐式启动（acquirePIDLock 同一 pidfile）:
//   MUST 恰一个成功（O_EXCL 原子互斥）
//   MUST 败者错误为 EEXIST 族（O_EXCL 指纹——flock 不会产生 EEXIST）
//   MUST pid 文件内容为 PID 行格式（单一实例记录）
func TestImplicitStart_ConcurrentSpawn(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "goalos.pid")

	type attempt struct {
		f   *os.File
		err error
	}
	const spawners = 2
	results := make(chan attempt, spawners)
	for i := 0; i < spawners; i++ {
		go func() {
			f, err := acquirePIDLock(pidPath)
			results <- attempt{f, err}
		}()
	}

	var winners, losers int
	var loserErrs []error
	for i := 0; i < spawners; i++ {
		a := <-results
		if a.err == nil {
			winners++
			if a.f == nil {
				t.Error("acquirePIDLock 成功但返回 nil 文件——实现异常")
			} else {
				a.f.Close()
			}
		} else {
			losers++
			loserErrs = append(loserErrs, a.err)
		}
	}

	if winners != 1 {
		t.Errorf("并发隐式启动互斥失效: %d/%d 个启动方获得锁——必须恰 1 个（O_EXCL pidfile 语义 R-1326），实际 winners=%d", winners, spawners, winners)
	}
	if losers != spawners-1 {
		t.Errorf("败者数异常: losers=%d（期望 %d，R-1326 单实例互斥）", losers, spawners-1)
	}
	// O_EXCL 指纹: 败者错误必须为 EEXIST 族。flock 下双方都能 open 成功——
	// 无 EEXIST 即非 O_EXCL 互斥（R-1326 "非 flock"）。
	for _, err := range loserErrs {
		if !errors.Is(err, fs.ErrExist) {
			t.Errorf("败者错误必须为 EEXIST 族（O_EXCL 判定指纹，R-1326 禁止 flock）: %v", err)
		}
	}

	// pid 文件内容: 单一实例 PID 行格式
	data, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatalf("pid 文件未写入: %v", err)
	}
	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil || pid <= 0 {
		t.Errorf("pid 文件内容必须为 PID 整数（实际 %q），解析失败: %v", string(data), err)
	}
}
