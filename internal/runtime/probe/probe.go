// Package probe 原生探针（R-1666——脚本运行时禁令：PowerShell/cmd 探针全退役，
// 企业组策略 ExecutionPolicy/AppLocker/CLM 会拦脚本=探针自崩≠边界生效）。
//
// 纯 Go 直调 syscall，输出数字 errno 证据（不随显示语言变化——GBK 本地化文本
// 匹配打地鼠问题同刀消灭）：
//
//	__goalos-probe write <path>   —— 写探针（os.WriteFile）
//	__goalos-probe read  <path>   —— 读探针（os.ReadFile）
//	__goalos-probe dial  <addr>   —— 网络探针（net.Dial tcp）
//
// 输出协议（双平台同构）：成功=打印 "PROBE-ERRNO=0" exit 0（泄漏面）；
// 拒绝=打印 "PROBE-ERRNO=<n>" exit 1（n=OS 错误码——Windows 5=AccessDenied/
// Linux 13=EACCES/WSAEACCES=10013 族）。
package probe

import (
	"fmt"
	"net"
	"os"
	"syscall"
	"time"

	"github.com/goalos/goalos/internal/fd3"
)

// ErrnoFromError 解开错误链取底层 errno（数字证据——跨平台 syscall.Errno）。
func ErrnoFromError(err error) int {
	for err != nil {
		if errno, ok := err.(syscall.Errno); ok {
			return int(errno)
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			break
		}
		err = u.Unwrap()
	}
	return -1
}

// Main 探针入口（进程模式——调用方=daemon 自 re-exec / 测试二进制 TestMain）。
// 返回 exit code：0=操作成功（边界缺席面），1=拒绝/失败。
func Main(args []string) int {
	if len(args) < 2 {
		fmt.Println("PROBE-ERRNO=-2 usage: __goalos-probe <write|read|dial> <target>")
		return 1
	}
	var err error
	switch args[0] {
	case "write":
		err = os.WriteFile(args[1], []byte("goalos-probe"), 0644)
	case "read":
		_, err = os.ReadFile(args[1])
	case "dial":
		var conn net.Conn
		conn, err = net.DialTimeout("tcp", args[1], 3*time.Second)
		if err == nil {
			conn.Close()
		}
	case "sleep":
		// 生命周期测试探针（Job 绞杀验证——长活子进程面）
		ms, err := time.ParseDuration(args[1] + "ms")
		if err != nil {
			// 合成码负数域（R-1670）：-3=探针参数无效——严禁借用 OS 正整数
			// errno 空间（POSIX EINVAL=22/Windows ERROR_INVALID_PARAMETER=87 全退役）。
			fmt.Println("PROBE-ERRNO=-3")
			return 1
		}
		time.Sleep(ms)
		fmt.Println("PROBE-SLEPT")
		return 0
	case "roundtrip":
		// 回环中继探针（FD3——R-1650 v2/R-1660 v2 证据形态）：dial+写+读回显——
		// 字节级全链验证（dial 探针只证连通；roundtrip 证中继真实通数据）。
		// 成功=PROBE-ERRNO=0 + ROUNDTRIP-OK；失败=PROBE-ERRNO=<n>。
		conn, derr := net.DialTimeout("tcp", args[1], 3*time.Second)
		if derr != nil {
			fmt.Printf("PROBE-ERRNO=%d\n", ErrnoFromError(derr))
			return 1
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		nonce := fmt.Sprintf("rt-%d", time.Now().UnixNano())
		if _, werr := conn.Write([]byte(nonce)); werr != nil {
			fmt.Printf("PROBE-ERRNO=%d\n", ErrnoFromError(werr))
			return 1
		}
		buf := make([]byte, 64)
		n, rerr := conn.Read(buf)
		if rerr != nil {
			fmt.Printf("PROBE-ERRNO=%d\n", ErrnoFromError(rerr))
			return 1
		}
		if string(buf[:n]) != nonce {
			fmt.Println("PROBE-ERRNO=-4") // 合成码：回显内容失真（中继中间人嫌疑面）
			return 1
		}
		fmt.Println("PROBE-ERRNO=0")
		fmt.Println("ROUNDTRIP-OK")
		return 0
	case "fd3rt":
		// unix socket FD3 帧中继探针（Linux 模式 B 面——R-1650 v2 S4）：
		// fd3rt <sockpath> <endpoint>——dial unix+OPEN 帧+OPEN_OK 校验+字节回显。
		// 成功=PROBE-ERRNO=0 + ROUNDTRIP-OK；broker 拒绝=PROBE-ERRNO=-5；
		// 传输出错=PROBE-ERRNO=<n>；回显失真=PROBE-ERRNO=-4。
		if len(args) < 3 {
			fmt.Println("PROBE-ERRNO=-3") // 参数不足
			return 1
		}
		conn, derr := fd3.Dial(args[1])
		if derr != nil {
			fmt.Printf("PROBE-ERRNO=%d\n", ErrnoFromError(derr))
			return 1
		}
		defer conn.Close()
		if werr := conn.WriteFrame(fd3.Frame{Op: fd3.OpOpen, Payload: []byte(args[2])}); werr != nil {
			fmt.Printf("PROBE-ERRNO=%d\n", ErrnoFromError(werr))
			return 1
		}
		resp, rerr := conn.ReadFrame()
		if rerr != nil {
			fmt.Printf("PROBE-ERRNO=%d\n", ErrnoFromError(rerr))
			return 1
		}
		if resp.Op != fd3.OpOpenOK {
			fmt.Println("PROBE-ERRNO=-5") // 合成码：broker 拒绝（治理否定面）
			return 1
		}
		nonce := fmt.Sprintf("rt-%d", time.Now().UnixNano())
		if werr := conn.WriteFrame(fd3.Frame{Op: fd3.OpData, Payload: []byte(nonce)}); werr != nil {
			fmt.Printf("PROBE-ERRNO=%d\n", ErrnoFromError(werr))
			return 1
		}
		echo, rerr := conn.ReadFrame()
		if rerr != nil {
			fmt.Printf("PROBE-ERRNO=%d\n", ErrnoFromError(rerr))
			return 1
		}
		if echo.Op != fd3.OpData || string(echo.Payload) != nonce {
			fmt.Println("PROBE-ERRNO=-4") // 回显失真
			return 1
		}
		fmt.Println("PROBE-ERRNO=0")
		fmt.Println("ROUNDTRIP-OK")
		return 0
	default:
		fmt.Println("PROBE-ERRNO=-2 unknown op")
		return 1
	}
	if err == nil {
		fmt.Println("PROBE-ERRNO=0")
		return 0
	}
	fmt.Printf("PROBE-ERRNO=%d\n", ErrnoFromError(err))
	return 1
}

// IsDenied 判定探针输出=拒绝（数字 errno 族——非零即拒；文本零依赖）。
func IsDenied(output string) (denied bool, errno int) {
	var n int
	if _, err := fmt.Sscanf(output, "PROBE-ERRNO=%d", &n); err != nil {
		return false, -1
	}
	return n != 0, n
}
