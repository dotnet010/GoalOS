// bypass.go——旁路测试防误计断言框架（TC-RT-002——R-1481①，两外部顾问独立收敛条款）：
// 验证「内核/虚拟化边界拒绝直接系统调用」时，能力代理拒绝经协议发起的请求
// 不得误计为旁路测试通过。两类防线分开测、分开计数。
package runtime

// BypassCounters 旁路测试计数器（两类防线分离——代理拒绝≠边界拒绝≠旁路通过）。
type BypassCounters struct {
	ProxyRefusals    int // 能力代理层拒绝（协议内请求被代理规则拒绝——非 OS 边界证据）
	BoundaryRefusals int // OS 边界拒绝（直接系统调用被内核/虚拟化边界拦截——旁路防线证据）
	bypassPasses     int // 旁路测试通过（仅边界拒绝可计入）
}

// NewBypassCounters 构造计数器。
func NewBypassCounters() *BypassCounters { return &BypassCounters{} }

// RecordProxyRefusal 登记代理层拒绝（协议内请求被拒——不计入旁路通过）。
func (c *BypassCounters) RecordProxyRefusal(what string) { c.ProxyRefusals++ }

// RecordBoundaryRefusal 登记 OS 边界拒绝（直接系统调用被拦——旁路防线命中）。
func (c *BypassCounters) RecordBoundaryRefusal(what string) { c.BoundaryRefusals++ }

// RecordBypassPass 登记旁路通过（唯一合法来源=边界拒绝确认后由测试显式登记）。
func (c *BypassCounters) RecordBypassPass() { c.bypassPasses++ }

// BypassPasses 旁路通过计数（核心不变量：代理拒绝不影响此值——TC-RT-002）。
func (c *BypassCounters) BypassPasses() int { return c.bypassPasses }
