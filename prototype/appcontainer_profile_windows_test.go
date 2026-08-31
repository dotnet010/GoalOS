//go:build prototype && windows

// appcontainer_profile_windows_test.go——顾问疑虑③（Profile 生命周期/残留泄漏）
// 实机裁决（2026-08-31）：删活竞争的错误码实锤 + Job KILL_ON_JOB_CLOSE 绞杀 +
// 退避重试删除=收敛性实证。裁决顾问推荐的修复链是否必要且充分。
package prototype

import (
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// deleteProfileE 返回 HRESULT 语义的删除（DeleteAppContainerProfile 返回
// HRESULT——S_OK=0 为成功，负值=失败；v1 误判成 BOOL 反置成败，v2 修正）。
// 返回 nil=成功；失败返回带 HRESULT 的错误。
func deleteProfileE(profile string) error {
	profilePtr, _ := windows.UTF16PtrFromString(profile)
	r1, _, _ := procDeleteAppContainerProfile.Call(uintptr(unsafe.Pointer(profilePtr)))
	hr := int32(r1) // HRESULT 有效位在低 32 位
	if hr >= 0 {
		return nil
	}
	return syscall.Errno(uint32(hr)) // 保留原始 HRESULT 值（如 0x80070005）
}

// TestAppContainer_ProfileLifecycle 疑虑③裁决。
func TestAppContainer_ProfileLifecycle(t *testing.T) {
	const profile = "GoalOS-Spike-AC-Life"
	_ = deleteProfileE(profile) // 清场

	base := t.TempDir()
	sleeper := buildBinary(t, base, "sleeper", sleeperSource)

	sid := createAppContainer(t, profile)

	// —— P1. 活进程持 profile 时删除：机器作答，不预设（顾问预言=SHARING_VIOLATION/ACCESS_DENIED）——
	pi := spawnInAppContainer(t, sid, `"`+sleeper+`"`, base+`\sleeper.out`)
	time.Sleep(1500 * time.Millisecond) // 让子进程真实跑起来（profile hive 加载）
	delErr := deleteProfileE(profile)
	if delErr == nil {
		t.Logf("P1 实机：活进程持 profile 时删除=成功（HRESULT S_OK）——顾问预警的句柄锁阻断在本构建（26200）未出现；残留风险面=崩溃未来得及删，非删除被阻塞")
	} else {
		t.Logf("P1 实机：活进程持 profile 删除=失败（HRESULT=0x%08X）——顾问预警机制实锤", uint32(int32(delErr.(syscall.Errno))))
	}

	// —— P2. Job KILL_ON_JOB_CLOSE 绞杀链：assign→close job→子进程死 → 退避重试删除收敛 ——
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		killAndWait(t, pi)
		t.Fatalf("CreateJobObject: %v", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		killAndWait(t, pi)
		windows.CloseHandle(job)
		t.Fatalf("SetInformationJobObject: %v", err)
	}
	if err := windows.AssignProcessToJobObject(job, pi.Process); err != nil {
		killAndWait(t, pi)
		windows.CloseHandle(job)
		t.Fatalf("AssignProcessToJobObject: %v", err)
	}
	// 关 job 句柄=内核级绞杀全部关联进程
	windows.CloseHandle(job)
	wait, _ := windows.WaitForSingleObject(pi.Process, 10000)
	if wait != 0 {
		killAndWait(t, pi)
		t.Fatalf("P2：Job 关闭后子进程未死——KILL_ON_JOB_CLOSE 未生效: wait=%d", wait)
	}
	var exitCode uint32
	_ = windows.GetExitCodeProcess(pi.Process, &exitCode)
	t.Logf("P2 Job 绞杀=生效（子进程 exit=%d）", exitCode)

	// —— P3. 退避重试删除：终态不变量=profile 确已消失（删除成功或重复删除返回未找到码）——
	var attempts int
	var lastErr error
	for attempts = 1; attempts <= 10; attempts++ {
		lastErr = deleteProfileE(profile)
		if lastErr == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if lastErr != nil {
		// 已删=重复删除的错误码是可接受终态（P1 若已删则此处必走此分支）
		t.Logf("P3：删除持续返回 HRESULT=0x%08X（若 P1 已删=预期内幂等面；若非=残留实锤）", uint32(int32(lastErr.(syscall.Errno))))
	} else {
		t.Logf("P3 退避重试删除=收敛（第 %d 次成功）", attempts)
	}

	// —— P4. 终态核验：重建同名 profile 成功=旧 profile 确已消失（最强不变量）——
	sid2 := createAppContainer(t, profile)
	windows.FreeSid(sid2)
	if err := deleteProfileE(profile); err != nil {
		t.Fatalf("P4：重建后删除失败: %v", err)
	}
	t.Logf("P4 终态核验=通过（重建+再删成功——profile 生命周期闭环无泄漏）")
}
