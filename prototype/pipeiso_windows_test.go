//go:build prototype && windows

// pipeiso_windows_test.go——命名管道最小回显隔离测试（forwarder 全链卡死的
// 归因二分：管道参数本身是否成立——无网络无子进程纯管道双工）。
package prototype

import (
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// TestPipeIso 最小管道回显：server(Connect→Read→Write 回显) ↔ client(Write→Read)。
func TestPipeIso(t *testing.T) {
	const pipeName = `\\.\pipe\GoalOS-PipeIso-2026`
	pipePtr, _ := windows.UTF16PtrFromString(pipeName)
	hPipe, err := windows.CreateNamedPipe(pipePtr, windows.PIPE_ACCESS_DUPLEX,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT,
		1, 4096, 4096, 0, nil)
	if err != nil {
		t.Fatalf("CreateNamedPipe: %v", err)
	}
	defer windows.CloseHandle(hPipe)

	go func() {
		if err := windows.ConnectNamedPipe(hPipe, nil); err != nil {
			t.Logf("server: ConnectNamedPipe: %v", err)
			return
		}
		t.Logf("server: connected")
		buf := make([]byte, 64)
		var n uint32
		if err := windows.ReadFile(hPipe, buf, &n, nil); err != nil {
			t.Logf("server: ReadFile: %v", err)
			return
		}
		t.Logf("server: read %d bytes", n)
		var w uint32
		if err := windows.WriteFile(hPipe, buf[:n], &w, nil); err != nil {
			t.Logf("server: WriteFile: %v", err)
			return
		}
		t.Logf("server: echoed %d bytes", w)
	}()

	time.Sleep(500 * time.Millisecond) // 等 server 进入 ConnectNamedPipe
	hClient, err := windows.CreateFile(pipePtr, windows.GENERIC_READ|windows.GENERIC_WRITE,
		0, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatalf("client CreateFile: %v", err)
	}
	defer windows.CloseHandle(hClient)
	t.Logf("client: opened")
	var w uint32
	if err := windows.WriteFile(hClient, []byte("ping-01"), &w, nil); err != nil {
		t.Fatalf("client WriteFile: %v", err)
	}
	t.Logf("client: wrote %d bytes", w)
	buf := make([]byte, 64)
	var n uint32
	if err := windows.ReadFile(hClient, buf, &n, nil); err != nil {
		t.Fatalf("client ReadFile: %v", err)
	}
	t.Logf("client: read back %d bytes=%q", n, string(buf[:n]))
	if string(buf[:n]) != "ping-01" {
		t.Fatalf("回显内容不符: %q", buf[:n])
	}
}
