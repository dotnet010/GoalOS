// isolation.go——I 族隔离强度 typed 枚举（R-1628③ 会议 #251 评审注记落地——
// 替代 W1 字符串比较（类型脆弱）；D-13 纪律：枚举显式赋值从 1 开始——零值非法）。
// 定义权威=06 §1.3（I0-I5 阶梯唯一定义 R-1506：按边界类型单维）。
package runtime

// IsolationLevel I 族隔离强度（零值非法——显式赋值从 1 开始）。
type IsolationLevel int

const (
	// I0 无隔离（值=1——零值非法纪律，D-13）
	I0 IsolationLevel = iota + 1
	I1                // 进程边界+资源限制
	I2                // OS 原生强制单面（syscall 过滤族或文件系统禁闭族任一）
	I3                // 双面齐备
	I4                // 独立内核边界（gVisor 用户态内核/Windows Sandbox/HV 微虚拟机）
	I5                // 一次性执行环境（v0.3.1 未实现——R-1602 显式拒绝）
)

// String wire 值（07 事件载荷/日志呈现用——I 族机器值）。
func (l IsolationLevel) String() string {
	switch l {
	case I0:
		return "I0"
	case I1:
		return "I1"
	case I2:
		return "I2"
	case I3:
		return "I3"
	case I4:
		return "I4"
	case I5:
		return "I5"
	default:
		return "INVALID"
	}
}

// ParseIsolationLevel wire 值→typed（非法值=I0 不返回——fail-closed 返回错误）。
func ParseIsolationLevel(s string) (IsolationLevel, error) {
	switch s {
	case "I0":
		return I0, nil
	case "I1":
		return I1, nil
	case "I2":
		return I2, nil
	case "I3":
		return I3, nil
	case "I4":
		return I4, nil
	case "I5":
		return I5, nil
	default:
		return 0, errInvalidIsolation(s)
	}
}

// ExecutionTier 执行分级 T 族（05 §X.6.2——与 I 族两轴正交；显式赋值从 1 开始）。
type ExecutionTier int

const (
	// TierT0 协作档（值=1——零值非法）
	TierT0 ExecutionTier = iota + 1
	TierRestricted     // T1 受限档
	TierHardenedLocal  // T2 强隔离本地档
	TierHardenedRemote // T3 强隔离远程档（v0.3.1 不做——R-1483）
)

// String wire 值（07 RuntimeSelected.tier=T0|T1|T2|T3）。
func (t ExecutionTier) String() string {
	switch t {
	case TierT0:
		return "T0"
	case TierRestricted:
		return "T1"
	case TierHardenedLocal:
		return "T2"
	case TierHardenedRemote:
		return "T3"
	default:
		return "INVALID"
	}
}
