// errors.go——runtime 包错误值（09 RTM 族——R-1620 注记：随任务 3.2 注册）。
package runtime

import "fmt"

// ErrInvalidIsolation I 族非法值（fail-closed——宁缺毋滥，不静默归 I0）。
type errInvalidIsolationType struct{ value string }

func (e *errInvalidIsolationType) Error() string {
	return fmt.Sprintf("runtime: 非法 IsolationLevel 值 %q（合法=I0~I5）", e.value)
}

func errInvalidIsolation(s string) error { return &errInvalidIsolationType{value: s} }
