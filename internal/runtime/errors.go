// errors.go——runtime 包错误值（09 RTM 族——RTM 族 W0 已注册（R-1491——09 §2 七码）；R-1620 注记修正（会议 #260 F3——原注记『随任务 3.2 注册』陈旧））。
package runtime

import "fmt"

// ErrInvalidIsolation I 族非法值（fail-closed——宁缺毋滥，不静默归 I0）。
type errInvalidIsolationType struct{ value string }

func (e *errInvalidIsolationType) Error() string {
	return fmt.Sprintf("runtime: 非法 IsolationLevel 值 %q（合法=I0~I5）", e.value)
}

func errInvalidIsolation(s string) error { return &errInvalidIsolationType{value: s} }
