//go:build windows

// transport_windows.go——FD3 Windows 传输层：命名管道双单向对（R-1662 v2——
// Req/Resp 各一独立句柄=同步句柄全双工互卡根治形态）+SDDL 授 AC 包面
//（D:P(A;;GA;;;WD)(A;;GA;;;AC)——spike 实证：仅 Everyone=AC 被拒；AAP 必需）。
// 名=熵化一次性（R-1667 v2 同族：GoalOS-FD3-<label>-<64bit 熵>——复用=残留态继承面）。
package fd3

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// fd3PipeSDDL 管道安全描述符（D:P=受保护 DACL；GA 授 Everyone+ALL APPLICATION
// PACKAGES——AC 内进程连宿主管道的法定面，spike 实机实证）。
const fd3PipeSDDL = "D:P(A;;GA;;;WD)(A;;GA;;;AC)"

var procSDDLToSD = windows.NewLazySystemDLL("advapi32.dll").NewProc("ConvertStringSecurityDescriptorToSecurityDescriptorW")
var procWaitNamedPipe = windows.NewLazySystemDLL("kernel32.dll").NewProc("WaitNamedPipeW")

// waitNamedPipe 等管道实例可用（x/sys 未包——手卷；返回 false=超时/失败）。
func waitNamedPipe(name *uint16, timeoutMs uint32) bool {
	r1, _, callErr := procWaitNamedPipe.Call(uintptr(unsafe.Pointer(name)), uintptr(timeoutMs))
	if r1 == 0 && callErr != nil && callErr != syscall.Errno(0) {
		return false // 错误=不可用面（超时 ERROR_SEM_TIMEOUT 族同归 false）
	}
	return r1 != 0
}

// fd3SecurityAttributes 管道 SA（SDDL→SD 一次性转换）。
func fd3SecurityAttributes() (*windows.SecurityAttributes, error) {
	sddlPtr, err := windows.UTF16PtrFromString(fd3PipeSDDL)
	if err != nil {
		return nil, err
	}
	var sd *windows.SECURITY_DESCRIPTOR
	r1, _, callErr := procSDDLToSD.Call(uintptr(unsafe.Pointer(sddlPtr)), 1, uintptr(unsafe.Pointer(&sd)), 0)
	if r1 == 0 {
		return nil, fmt.Errorf("fd3: SDDL→SD 失败: %v", callErr)
	}
	return &windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: sd,
		InheritHandle:      0,
	}, nil
}

// Listener FD3 监听器（基名熵化；Accept=每连接开新管道对实例——命名管道多实例语义）。
type Listener struct {
	base    string
	sa      *windows.SecurityAttributes
	mu      sync.Mutex
	closed  bool
	pending []windows.Handle // 在飞 ConnectNamedPipe 句柄（Close 打断用）
}

// ErrListenerClosed 监听器已关（Accept 退出信号）。
var ErrListenerClosed = fmt.Errorf("fd3: 监听器已关闭")

// Name 管道对基名（Dial 侧寻址用）。
func (l *Listener) Name() string { return l.base }

func (l *Listener) track(hs ...windows.Handle) {
	l.mu.Lock()
	l.pending = append(l.pending, hs...)
	l.mu.Unlock()
}

func (l *Listener) untrack(hs ...windows.Handle) {
	l.mu.Lock()
	for _, h := range hs {
		for i, p := range l.pending {
			if p == h {
				l.pending = append(l.pending[:i], l.pending[i+1:]...)
				break
			}
		}
	}
	l.mu.Unlock()
}

// Listen 建监听器（label 净化+熵后缀——同标签两次 Listen 必异名）。
// dir 参数=跨平台签名对称位（unix=socket 落目录；windows=管道无目录概念——忽略）。
func Listen(_ string, label string) (*Listener, error) {
	var entropy [8]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return nil, fmt.Errorf("fd3: 熵源失败: %w", err)
	}
	sa, err := fd3SecurityAttributes()
	if err != nil {
		return nil, err
	}
	return &Listener{
		base: fmt.Sprintf(`\\.\pipe\GoalOS-FD3-%s-%s`, sanitizeLabel(label), hex.EncodeToString(entropy[:])),
		sa:   sa,
	}, nil
}

// Close 监听器收尾：翻 closed+CancelIoEx 打断全部在飞 ConnectNamedPipe
//（Accept 孤儿 goroutine 防泄漏——实机实证：同步 ConnectNamedPipe 无打断=僵尸悬挂）。
func (l *Listener) Close() error {
	l.mu.Lock()
	l.closed = true
	pending := append([]windows.Handle{}, l.pending...)
	l.mu.Unlock()
	for _, h := range pending {
		_ = windows.CancelIoEx(h, nil) // 打断在飞连接等待（错误=已完成——幂等）
	}
	return nil
}

// Accept 接受一连接（建 req/resp 双管道实例+等待客户端双连）。
// req=客户端→服务端（服务端读）；resp=服务端→客户端（服务端写）。
// ERROR_PIPE_CONNECTED=客户端在 Create 与 Connect 之间抢连=连接已成（非失败——
// 实机实证竞态：误当失败=Accept 循环早夭=后续 dial 全超时）。
// Close 联动：CancelIoEx 打断在飞 ConnectNamedPipe（防 Accept 孤儿 goroutine 泄漏）。
func (l *Listener) Accept() (*Conn, error) {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil, ErrListenerClosed
	}
	l.mu.Unlock()

	reqPtr, err := windows.UTF16PtrFromString(l.base + "-req")
	if err != nil {
		return nil, err
	}
	respPtr, err := windows.UTF16PtrFromString(l.base + "-resp")
	if err != nil {
		return nil, err
	}
	hReq, err := windows.CreateNamedPipe(reqPtr,
		windows.PIPE_ACCESS_INBOUND|windows.FILE_FLAG_OVERLAPPED,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT,
		windows.PIPE_UNLIMITED_INSTANCES, 64*1024, 64*1024, 0, l.sa)
	if err != nil {
		return nil, fmt.Errorf("fd3: CreateNamedPipe req: %w", err)
	}
	hResp, err := windows.CreateNamedPipe(respPtr,
		windows.PIPE_ACCESS_OUTBOUND|windows.FILE_FLAG_OVERLAPPED,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT,
		windows.PIPE_UNLIMITED_INSTANCES, 64*1024, 64*1024, 0, l.sa)
	if err != nil {
		windows.CloseHandle(hReq)
		return nil, fmt.Errorf("fd3: CreateNamedPipe resp: %w", err)
	}
	l.track(hReq, hResp)
	defer l.untrack(hReq, hResp)
	for _, h := range []windows.Handle{hReq, hResp} {
		if err := l.connectOne(h); err != nil {
			windows.CloseHandle(hReq)
			windows.CloseHandle(hResp)
			return nil, err
		}
	}
	return &Conn{r: hReq, w: hResp}, nil
}

// connectOne 单管道等待连接（overlapped+事件——Close 可 CancelIoEx 打断）。
func (l *Listener) connectOne(h windows.Handle) error {
	evt, err := windows.CreateEvent(nil, 1, 0, nil) // manual-reset
	if err != nil {
		return err
	}
	defer windows.CloseHandle(evt)
	var ov windows.Overlapped
	ov.HEvent = evt
	err = windows.ConnectNamedPipe(h, &ov)
	if err == nil || err == windows.ERROR_PIPE_CONNECTED {
		return nil // 同步即连/抢连已成
	}
	if err != windows.ERROR_IO_PENDING {
		return fmt.Errorf("fd3: ConnectNamedPipe: %w", err)
	}
	if _, err := windows.WaitForSingleObject(evt, windows.INFINITE); err != nil {
		return fmt.Errorf("fd3: 等连接事件: %w", err)
	}
	l.mu.Lock()
	closed := l.closed
	l.mu.Unlock()
	if closed {
		return ErrListenerClosed
	}
	// overlapped 完结确认（CancelIoEx 打断路径=GetOverlappedResult 失败=正常退出面）
	var n uint32
	if err := windows.GetOverlappedResult(h, &ov, &n, false); err != nil {
		return fmt.Errorf("fd3: 连接收尾: %w", err)
	}
	return nil
}

// Dial 客户端连接（req=写，resp=读——与服务端对偶）。
// 实例竞态纪律（实机实证）：WaitNamedPipe 返回≠CreateFile 成功——两客户端间
// 存在抢夺窗口（PIPE_BUSY）；每根管道=wait+create 重试环直到 10s 预算耗尽。
func Dial(base string) (*Conn, error) {
	dialOne := func(name string, access uint32) (windows.Handle, error) {
		namePtr, err := windows.UTF16PtrFromString(name)
		if err != nil {
			return 0, err
		}
		deadline := time.Now().Add(10 * time.Second)
		for {
			if !waitNamedPipe(namePtr, 2000) && time.Now().After(deadline) {
				return 0, fmt.Errorf("fd3: 等管道超时 %s", name)
			}
			h, err := windows.CreateFile(namePtr, access, 0, nil, windows.OPEN_EXISTING, 0, 0)
			if err == nil {
				return h, nil
			}
			if err == windows.ERROR_PIPE_BUSY || err == windows.ERROR_FILE_NOT_FOUND {
				if time.Now().After(deadline) {
					return 0, fmt.Errorf("fd3: 管道忙重试耗尽 %s", name)
				}
				time.Sleep(20 * time.Millisecond)
				continue
			}
			return 0, fmt.Errorf("fd3: 连管道失败 %s: %w", name, err)
		}
	}
	hReq, err := dialOne(base+"-req", windows.GENERIC_WRITE)
	if err != nil {
		return nil, err
	}
	hResp, err := dialOne(base+"-resp", windows.GENERIC_READ)
	if err != nil {
		windows.CloseHandle(hReq)
		return nil, err
	}
	return &Conn{r: hResp, w: hReq}, nil
}

// Conn 单连接（读/写句柄独立——并发双工安全面的结构前提）。
type Conn struct {
	r windows.Handle
	w windows.Handle
}

// ReadFrame 读帧（读句柄）。
func (c *Conn) ReadFrame() (Frame, error) {
	return ReadFrame(&handleReader{c.r})
}

// WriteFrame 写帧（写句柄）。
func (c *Conn) WriteFrame(f Frame) error {
	return WriteFrame(&handleWriter{c.w}, f)
}

// Close 关双句柄。
func (c *Conn) Close() error {
	windows.CloseHandle(c.r)
	windows.CloseHandle(c.w)
	return nil
}

// handleReader/handleWriter windows.Handle→io 适配（同步 ReadFile/WriteFile）。
type handleReader struct{ h windows.Handle }

func (r *handleReader) Read(p []byte) (int, error) {
	var n uint32
	err := windows.ReadFile(r.h, p, &n, nil)
	if err != nil {
		if err == syscall.ERROR_BROKEN_PIPE || err == windows.ERROR_HANDLE_EOF {
			return int(n), fmt.Errorf("fd3: 管道关闭: %w", err)
		}
		return int(n), err
	}
	return int(n), nil
}

type handleWriter struct{ h windows.Handle }

func (w *handleWriter) Write(p []byte) (int, error) {
	var n uint32
	err := windows.WriteFile(w.h, p, &n, nil)
	if err != nil {
		return int(n), err
	}
	return int(n), nil
}
