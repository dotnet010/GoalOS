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
			fmt.Println("PROBE-ERRNO=22") // EINVAL——参数非数字
			return 1
		}
		time.Sleep(ms)
		fmt.Println("PROBE-SLEPT")
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
