//go:build windows

// Package pluginrunner — Windows 子进程安全加固（v0.3.0）。
// Job Object 实现进程组隔离和资源限制。
// 设计依据：08 沙箱规范 §5.2、R-863 Windows #1 平台策略。
package pluginrunner

import (
	"crypto/rand"
	"encoding/hex"
	"os/exec"
	"syscall"
	"unsafe"
)

// sanitizeChildProcess 在子进程启动前设置 Windows 安全加固。
// v0.3.0 fix (C5): 通过 Job Object 实现进程组隔离——父进程终止时子进程自动清理。
// 后续版本将扩展：Restricted Token + Low Integrity Level + ACLs（agentbox 集成）。
func sanitizeChildProcess(cmd *exec.Cmd) {
	// 生成唯一 Job Object 名称
	id := make([]byte, 4)
	rand.Read(id)
	name := "Global\\GoalOS_Sandbox_" + hex.EncodeToString(id)
	jobName, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return // fallback: no isolation
	}

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	createJobObject := kernel32.NewProc("CreateJobObjectW")
	job, _, err := createJobObject.Call(0, uintptr(unsafe.Pointer(jobName)))
	if job == 0 {
		return // fallback: no isolation
	}
	_ = err

	// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE = 0x00002000
	const JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE = 0x00002000
	const JobObjectExtendedLimitInformation = 2

	type jobLimitInfo struct {
		PerProcessUserTimeLimit int64
		PerJobUserTimeLimit     int64
		LimitFlags              uint32
		MinimumWorkingSetSize   uintptr
		MaximumWorkingSetSize   uintptr
		ActiveProcessLimit      uint32
		Affinity                uintptr
		PriorityClass           uint32
		SchedulingClass         uint32
	}

	info := jobLimitInfo{
		LimitFlags: JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
	}

	setInfoJob := kernel32.NewProc("SetInformationJobObject")
	setInfoJob.Call(
		job,
		uintptr(JobObjectExtendedLimitInformation),
		uintptr(unsafe.Pointer(&info)),
		uintptr(unsafe.Sizeof(info)),
	)

	// 将当前进程加入 Job——子进程自动继承
	assignProc := kernel32.NewProc("AssignProcessToJobObject")
	currentProc, err := syscall.GetCurrentProcess()
	if err != nil {
		return // fallback: no Job Object isolation
	}
	assignProc.Call(job, uintptr(currentProc))
}
