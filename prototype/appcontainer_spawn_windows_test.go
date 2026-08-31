//go:build prototype && windows

// appcontainer_spawn_windows_test.go——异步拉起辅助（返回进程信息不等待——
// 供回环监听/生命周期等需要「容器内长活进程」场景的测试复用）。
// 与 runInAppContainer 同一拉起面（CREATE_SUSPENDED+ResumeThread 时序纪律同族）。
package prototype

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// spawnInAppContainer 在 AppContainer 内异步拉起进程（输出重定向至 outFile）。
// 返回进程句柄——调用方负责 killAndWait/关闭（t.Cleanup 兜底关闭句柄）。
func spawnInAppContainer(t *testing.T, sid *windows.SID, cmdline, outFile string) *windows.ProcessInformation {
	t.Helper()

	outPathPtr, _ := windows.UTF16PtrFromString(outFile)
	sa := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), InheritHandle: 1}
	hOut, err := windows.CreateFile(outPathPtr, windows.GENERIC_WRITE, windows.FILE_SHARE_READ, sa, windows.CREATE_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatalf("CreateFile 捕获文件: %v", err)
	}
	t.Cleanup(func() { windows.CloseHandle(hOut) })

	secCaps := securityCapabilities{appContainerSid: sid}
	attrList, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		t.Fatalf("NewProcThreadAttributeList: %v", err)
	}
	t.Cleanup(func() { attrList.Delete() })
	if err := attrList.Update(procThreadAttributeSecurityCapabilities, unsafe.Pointer(&secCaps), unsafe.Sizeof(secCaps)); err != nil {
		t.Fatalf("UpdateProcThreadAttribute SECURITY_CAPABILITIES: %v", err)
	}

	var siex windows.StartupInfoEx
	siex.Cb = uint32(unsafe.Sizeof(siex))
	siex.Flags = startfUseStdHandles
	siex.StdOutput = hOut
	siex.StdErr = hOut
	siex.ProcThreadAttributeList = attrList.List()

	cmdPtr, _ := windows.UTF16PtrFromString(cmdline)
	pi := &windows.ProcessInformation{}
	err = windows.CreateProcess(nil, cmdPtr, nil, nil, true,
		windows.CREATE_SUSPENDED|extendedStartupinfoPresent|windows.CREATE_UNICODE_ENVIRONMENT,
		nil, nil, &siex.StartupInfo, pi)
	if err != nil {
		t.Fatalf("CreateProcess(AppContainer spawn): %v", err)
	}
	t.Cleanup(func() {
		windows.CloseHandle(pi.Process)
		windows.CloseHandle(pi.Thread)
	})
	if _, err := windows.ResumeThread(pi.Thread); err != nil {
		t.Fatalf("ResumeThread: %v", err)
	}
	return pi
}

// spawnOutPath 标准输出捕获路径分配。
func spawnOutPath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(t.TempDir(), name)
}

// killAndWait 强杀+等待（生命周期测试用）；并冲刷读回辅助由调用方自理。
func killAndWait(t *testing.T, pi *windows.ProcessInformation) {
	t.Helper()
	_ = windows.TerminateProcess(pi.Process, 1)
	windows.WaitForSingleObject(pi.Process, 10000)
}

// readSpawnOut 读回捕获文件。
func readSpawnOut(path string) string {
	data, _ := os.ReadFile(path)
	return string(data)
}
