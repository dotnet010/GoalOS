//go:build prototype && windows

// pipeiso2_windows_test.go——全双工并发管道隔离（forwarder 卡死归因二分 v2：
// v1 单向回显=通；差异点=两端同句柄并发读写——同步句柄并发双工是否死锁）。
package prototype

import (
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// TestPipeIsoDuplex 两端并发双工：server(读goroutine+写循环) ↔ client(读主+写goroutine)。
func TestPipeIsoDuplex(t *testing.T) {
	const pipeName = `\\.\pipe\GoalOS-PipeIso2-2026`
	pipePtr, _ := windows.UTF16PtrFromString(pipeName)
	hPipe, err := windows.CreateNamedPipe(pipePtr, windows.PIPE_ACCESS_DUPLEX,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT,
		1, 4096, 4096, 0, nil)
	if err != nil {
		t.Fatalf("CreateNamedPipe: %v", err)
	}
	defer windows.CloseHandle(hPipe)

	serverDone := make(chan string, 1)
	go func() {
		if err := windows.ConnectNamedPipe(hPipe, nil); err != nil {
			serverDone <- "connect-fail"
			return
		}
		// 并发：goroutine 读，立即写一笔（不等读——模拟中继并发）
		readCh := make(chan int, 1)
		go func() {
			buf := make([]byte, 64)
			var n uint32
			if err := windows.ReadFile(hPipe, buf, &n, nil); err != nil {
				readCh <- -1
				return
			}
			readCh <- int(n)
		}()
		var w uint32
		if err := windows.WriteFile(hPipe, []byte("srv-hi"), &w, nil); err != nil {
			serverDone <- "srv-write-fail: " + err.Error()
			return
		}
		select {
		case n := <-readCh:
			if n <= 0 {
				serverDone <- "srv-read-fail"
				return
			}
			serverDone <- "ok"
		case <-time.After(5 * time.Second):
			serverDone <- "srv-read-timeout（读卡死实锤）"
		}
	}()

	time.Sleep(300 * time.Millisecond)
	hClient, err := windows.CreateFile(pipePtr, windows.GENERIC_READ|windows.GENERIC_WRITE,
		0, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatalf("client CreateFile: %v", err)
	}
	defer windows.CloseHandle(hClient)
	// 客户端并发：主写一笔，goroutine 读
	clientRead := make(chan string, 1)
	go func() {
		buf := make([]byte, 64)
		var n uint32
		if err := windows.ReadFile(hClient, buf, &n, nil); err != nil {
			clientRead <- "cli-read-fail"
			return
		}
		clientRead <- string(buf[:n])
	}()
	var w uint32
	if err := windows.WriteFile(hClient, []byte("cli-hi!"), &w, nil); err != nil {
		t.Fatalf("client WriteFile: %v", err)
	}
	t.Logf("client 已写")
	select {
	case got := <-clientRead:
		t.Logf("client 读到=%q", got)
	case <-time.After(5 * time.Second):
		t.Logf("client 读超时")
	}
	srvResult := <-serverDone
	t.Logf("server 结果=%s", srvResult)
	if srvResult != "ok" {
		t.Fatalf("全双工并发=死锁/失败: %s", srvResult)
	}
}
