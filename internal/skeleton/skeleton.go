// skeleton.go — Skeleton[T] 骨架期类型地位（R-1457 收编类型 1；R-1468 统一+方向性判别补丁；
// R-1473 合规检查 CI 机检化——会议 #222/#226）。
//
// 契约：骨架期类型地位=类型承载语义（非注释约定）：
//
//	Unwrap() (T, error)——未实现时返回 ErrNotImplemented（类型系统强制：不调用 Unwrap()
//	  根本拿不到裁决值）；
//	Direction() FailClosed|FailOpen——方向判别显式声明（fail-open 方向必须附安全评审签字）；
//	TaskRef() string——转绿任务引用（非注释——类型承载）。
//	SkeletonValue() T+Justification() string——多值枚举方向判别补丁（R-1468：显式声明
//	  选了哪个值+为什么——Direction() 二元字段不适合多值枚举）。
package skeleton

import "fmt"

// Direction — 骨架期失败方向判别（R-1457 类型 1 标准解法）。
type Direction string

const (
	FailClosed Direction = "FailClosed" // 安全方向（假设特性不可用/拒绝执行）
	FailOpen   Direction = "FailOpen"   // 危险方向（必须附安全评审签字——R-1457 纪律）
)

// Skeleton — 骨架期类型地位（R-1468 统一纪律）。
// 契约：未实现时 Unwrap() 返回 ErrNotImplemented——类型系统强制（不调用 Unwrap() 拿不到值）。
type Skeleton[T any] struct {
	value       T         // 骨架期值（未实现时=零值——Unwrap() 不返回）
	direction   Direction // 方向判别（FailClosed/FailOpen——fail-open 必须附安全评审签字）
	taskRef     string    // 转绿任务引用（R-xxx——非注释，类型承载）
	implemented bool      // 是否已实现（false=骨架期——Unwrap() 返回 ErrNotImplemented）
}

// NotImplemented — 构造骨架期 Skeleton[T]（未实现——Unwrap() 返回 ErrNotImplemented）。
// 契约：direction=方向判别显式声明；taskRef=转绿任务引用（R-xxx）。
func NotImplemented[T any](direction Direction, taskRef string) Skeleton[T] {
	return Skeleton[T]{
		direction:   direction,
		taskRef:     taskRef,
		implemented: false,
	}
}

// Implemented — 构造已实现 Skeleton[T]（转绿后——Unwrap() 返回真实值）。
func Implemented[T any](value T, direction Direction, taskRef string) Skeleton[T] {
	return Skeleton[T]{
		value:       value,
		direction:   direction,
		taskRef:     taskRef,
		implemented: true,
	}
}

// Unwrap — 解包（未实现时返回 ErrNotImplemented——类型系统强制）。
func (s Skeleton[T]) Unwrap() (T, error) {
	if !s.implemented {
		var zero T
		return zero, fmt.Errorf("skeleton: not implemented (task: %s, direction: %s)——ErrNotImplemented", s.taskRef, s.direction)
	}
	return s.value, nil
}

// Direction — 方向判别读取。
func (s Skeleton[T]) Direction() Direction { return s.direction }

// TaskRef — 转绿任务引用读取。
func (s Skeleton[T]) TaskRef() string { return s.taskRef }

// SkeletonValue — 骨架期具体值读取（R-1468 补丁——多值枚举方向判别：显式声明选了哪个值）。
// 契约：仅已实现时可用（implemented=true）——骨架期调用=panic（显式声明选了哪个值的前提=已实现）。
func (s Skeleton[T]) SkeletonValue() T {
	if !s.implemented {
		panic("skeleton: SkeletonValue() called on unimplemented skeleton——骨架期值不可信（R-1468）")
	}
	return s.value
}

// Justification — 方向判别理由读取（R-1468 补丁——为什么选这个值）。
// 契约：仅已实现时可用（implemented=true）。
func (s Skeleton[T]) Justification() string {
	if !s.implemented {
		panic("skeleton: Justification() called on unimplemented skeleton——骨架期无理由（R-1468）")
	}
	return s.taskRef // 骨架期 taskRef=理由载体
}
