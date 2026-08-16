// governance_authorize.go — 授权凭证类型（R-1448——发现 6 升维：治理闸门的编译期强制）。
//
// 契约（R-906a 治理不变量的编译期形态）："闸门之后调用"从时序约定升格为类型依赖——
// Execute 所需的 spec 参数类型（authorizedSpec）只能由 Governance 闸门通过后的函数返回。
// authorizedSpec 为未导出类型+包私有字段——pluginrunner 无法构造（编译期不存在非法路径）。
// 桥接不评估治理，只消费授权凭证（凭证=治理已过的编译期可验证形态）。
package governance

import "fmt"

// AuthorizedSpec — 授权后的执行规格（导出类型+包私有字段）。
// 字段包私有：spec 不可被外部读出后自行构造绕过（Pike 纪律——外部包只能调用
// Spec() 读取，不能构造该类型的值；唯一构造路径=Authorize()）。
type AuthorizedSpec struct {
	spec any // 执行规格（包私有——外部不可写）
}

// Authorize — 治理闸门通过后返回授权凭证（authorizedSpec 的唯一构造函数）。
// 契约：decision.Policy=="ALLOW" 且 decision.Capability=="GRANTED" 时才返回凭证；
// 否则返回 error（fail-closed——未授权=类型不存在=编译期不可达 Execute）。
func (e *Engine) Authorize(decision Decision, spec any) (AuthorizedSpec, error) {
	if decision.Policy != "ALLOW" || decision.Capability != "GRANTED" {
		return AuthorizedSpec{}, fmt.Errorf("governance: 未授权（Policy=%s Capability=%s）——授权凭证不存在（fail-closed）", decision.Policy, decision.Capability)
	}
	return AuthorizedSpec{spec: spec}, nil
}

// Spec — 授权凭证的执行规格读取（pluginrunner 调用——仅读，不可构造）。
// 返回类型为 any——调用方断言为具体类型（governance 不感知 spec 具体类型）。
func (a AuthorizedSpec) Spec() any { return a.spec }
