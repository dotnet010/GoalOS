// CLI 退出码契约（R-1379 / R-1323 / R-1383）。
//
// 契约（resolutions.yaml）:
//   R-1323: 审批命令形态含 "退出码 124/125"
//   R-1379: 125 = 审批未获通过 / guard 拦截（"125 分支 guard 族文案"）
//   R-1383: SDL 收口批——具名测试点名
//   → 124 = 超时；125 = 审批未获通过 / guard 拦截；125 ≠ 124。
//
// 先红状态（阶段 3.5 测试先行闸口——用例先红）:
//   internal/client 与 CLI main 均无退出码常量/映射（grep ExitCode/124/125
//   零命中；CLI 现状仅 os.Exit(1)/(2)）。
//   红锚: 反射探针——*client.Client 无退出码映射 API → t.Errorf 明确红。
//   断言体写全: 若未来映射 API 落地，按契约表精确断言 124/125 语义。
//
// 转绿任务: 7.23（C-2 表）——审批命令形态落地（R-1323/R-1379）后本测试转绿。
//
// 断言方式: 反射探针（公开 API 枚举）+ 行为断言（映射调用）——禁止读源码文本断言。
package main

import (
	"reflect"
	"testing"

	"github.com/goalos/goalos/internal/client"
)

// ─── 契约表（R-1323/R-1379 权威语义）───
const (
	contractExitApprovalDenied = 125 // 审批未获通过 / guard 拦截
	contractExitTimeout        = 124 // 超时
)

// TestCLI_ExitCode_125_Family — CLI 退出码契约。
//
// MUST 审批未获通过 / guard 拦截 → 退出码 125（R-1379）
// MUST 超时 → 退出码 124（R-1323）
// 互斥要求: 码位 125 与 124 语义互斥，不可复用（R-1323/R-1379）
//
// 先红探针: 映射 API 不存在 → 明确红（R-1379 先红）。
// 断言体写全: 候选映射 API 存在时执行精确断言（签名 + 值 + 互斥）。
func TestCLI_ExitCode_125_Family(t *testing.T) {
	c := client.New("http://127.0.0.1:1") // 探针载体——不发起真实请求
	ct := reflect.TypeOf(c)

	// ── 探针: 退出码映射 API（未来契约命名候选逐一探测）──
	var mapper reflect.Value
	var mapperName string
	for _, cand := range []string{"ExitCodeFor", "ExitCodeFromCause", "ExitCode"} {
		if m, ok := ct.MethodByName(cand); ok {
			mapper, mapperName = m.Func, cand
			break
		}
	}

	if !mapper.IsValid() {
		// 先红: 映射未实现——R-1379 红锚
		t.Errorf("CLI 退出码 124/125 映射未实现——R-1379 先红（internal/client 无 ExitCodeFor/ExitCodeFromCause/ExitCode 映射 API；CLI main 仅 os.Exit(1)/(2)）。转绿任务 7.23")
		return
	}

	// ── 断言体写全（映射存在时的精确断言）──
	// 签名约束: func(cause string) int
	mt := mapper.Type()
	if mt.NumIn() != 2 || mt.In(1).Kind() != reflect.String || mt.NumOut() != 1 || mt.Out(0).Kind() != reflect.Int {
		t.Errorf("退出码映射 API %s 签名不符合契约: 应 func(cause string) int，实际 %v", mapperName, mt)
		return
	}
	call := func(cause string) int {
		out := mapper.Call([]reflect.Value{reflect.ValueOf(c), reflect.ValueOf(cause)})
		return int(out[0].Int())
	}

	if got := call("approval_denied"); got != contractExitApprovalDenied {
		t.Errorf("审批未获通过必须退出 125（R-1379），映射 %s 返回 %d", mapperName, got)
	}
	if got := call("guard_intercepted"); got != contractExitApprovalDenied {
		t.Errorf("guard 拦截必须退出 125（R-1379），映射 %s 返回 %d", mapperName, got)
	}
	if got := call("timeout"); got != contractExitTimeout {
		t.Errorf("超时必须退出 124（R-1323），映射 %s 返回 %d", mapperName, got)
	}
	// 语义互斥: 125 与 124 不可复用同一码位
	if contractExitApprovalDenied == contractExitTimeout {
		t.Error("契约错误: 125（审批未获通过）与 124（超时）语义必须互斥（R-1323/R-1379）")
	}
}
