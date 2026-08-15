// Package errors 定义 GoalOS 系统错误码体系。
//
// v0.3.0 四段式编码（09 错误码知识库规范 §1 强制，R-1022）：
//
//	[域]-[子域]-[严重级]-[序号]——全局唯一且自带语义。
//	域：SBX(沙箱)/IPC(通信)/CTR(契约)/SKL(Skill)/CFG(配置)/GOV(治理)
//	子域：WIN(Windows)/LNX(Linux)/NET(网络)/FS(文件系统)/AUTH(鉴权)
//	严重级：F(Fatal)/R(Retryable)/W(Warning)/I(Info)——直接映射 R-943 四态
//	序号：001-999（同一子域+严重级下按发现顺序递增）
//
// 旧版三段式常量（GOAL_NOT_FOUND 等）保留为兼容层——新码一律四段式。
package errors

// ─── v0.3.0 四段式错误码（P0 覆盖清单——09 §2 条目）───

// SBX-WIN：沙箱域/Windows/Fatal
const (
	CodeSbxWinF001 Code = "SBX-WIN-F-001" // Windows 沙箱容器启动失败（Hyper-V 未启用）
)

// CTR：契约域
const (
	CodeCtrRecovI001      Code = "CTR-RECOV-I-001"         // 事件流恢复进行中（≥500ms）
	CodeCtrRecovI002      Code = "CTR-RECOV-I-002"         // 事件流恢复完成
	CodeCtrRecovF001      Code = "CTR-RECOV-F-001"         // 事件流恢复校验失败（状态被意外修改）
	CodeCtrWrTimeoutR002  Code = "CTR-WRITE-TIMEOUT-R-002" // 事件写入挂起超时
	CodeCtrWalCorruptF003 Code = "CTR-WAL-CORRUPT-F-003"   // 事件条目校验和失败
	CodeCtrFloW001        Code = "CTR-FLO-W-001"           // 无匹配执行模板（FLOW_TEMPLATE_MISSING）
)

// IPC：通信域
const (
	CodeIpcHndshkF001 Code = "IPC-HNDSHK-F-001" // 插件握手超时（双 nonce 转录绑定 5s 超时）
)

// GOV：治理域
const (
	CodeGovApxF001 Code = "GOV-APX-F-001" // 审批扩展次数耗尽（approval_extensions_exhausted）
	CodeGovLlmR001 Code = "GOV-LLM-R-001" // 模型响应超时（AgentErrorCode=TIMEOUT）
	CodeGovLlmF001 Code = "GOV-LLM-F-001" // 模型输出解析失败（AgentErrorCode=PARSE_ERROR）
)

// 旧版三段式常量保留在 errors.go（兼容层——新码一律四段式，旧码逐步迁移）。

// String — 错误码字符串。
func (c Code) String() string { return string(c) }

// Error — 实现 error 接口（四段式码的默认消息=码值本身；用户可见消息归 09 知识库条目）。
func (c Code) Error() string { return "[" + string(c) + "]" }
