//go:build linux && arm64

// 契约测试夹具（clone_flags_contract_test.go）：aarch64 裸 clone 入口。
// 语义与 rawclone_linux.go 注释一致——子进程路径直接跳入 cloneChildExit，
// 不触碰调用栈、不执行 Go 代码，因此 CLONE_VM/CLONE_THREAD 共享栈下安全。

#include "textflag.h"

// func rawCloneThread(flags uintptr) (tid uintptr, errno uintptr)
// clone(flags, 0, 0, 0, 0) —— 不提供 child stack：子进程路径不做任何栈操作。
TEXT ·rawCloneThread(SB),NOSPLIT,$0-24
	MOVD	flags+0(FP), R0
	MOVD	$0, R1
	MOVD	$0, R2
	MOVD	$0, R3
	MOVD	$0, R4
	MOVD	$220, R8 // SYS_CLONE (aarch64)
	SVC
	CMP	$0, R0
	BGE	ok
	// errno（负值）
	NEG	R0, R0
	MOVD	R0, errno+16(FP)
	MOVD	$0, tid+8(FP)
	RET
ok:
	CBNZ	R0, parent
	// 子进程：直接退出——不执行任何 Go/栈操作。
	JMP	·cloneChildExit(SB)
parent:
	MOVD	R0, tid+8(FP)
	MOVD	$0, errno+16(FP)
	RET

// cloneChildExit 以纯寄存器操作执行 SYS_EXIT（子进程夹具入口）。
TEXT ·cloneChildExit(SB),NOSPLIT,$0-0
	MOVD	$93, R8 // SYS_EXIT (aarch64)
	MOVD	$0, R0
	SVC
	WORD	$0x00000000 // UDF（不可达安全网）
