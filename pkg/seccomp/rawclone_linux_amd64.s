//go:build linux && amd64

// 契约测试夹具（clone_flags_contract_test.go）：x86_64 裸 clone 入口。
// 语义与 rawclone_linux.go 注释一致——子进程路径直接跳入 cloneChildExit，
// 不触碰调用栈、不执行 Go 代码，因此 CLONE_VM/CLONE_THREAD 共享栈下安全。

#include "textflag.h"

// func rawCloneThread(flags uintptr) (tid uintptr, errno uintptr)
// clone(flags, 0, 0, 0, 0) —— 不提供 child stack：子进程路径不做任何栈操作。
TEXT ·rawCloneThread(SB),NOSPLIT,$0-24
	MOVQ	flags+0(FP), DI
	XORQ	SI, SI
	XORQ	DX, DX
	XORQ	R10, R10
	XORQ	R8, R8
	MOVL	$56, AX // SYS_CLONE (x86_64)
	SYSCALL
	CMPQ	AX, $0xfffffffffffff001
	JAE	err
	TESTQ	AX, AX
	JNZ	parent
	// 子进程：直接退出——不执行任何 Go/栈操作。
	JMP	·cloneChildExit(SB)
parent:
	MOVQ	AX, tid+8(FP)
	MOVQ	$0, errno+16(FP)
	RET
err:
	NEGQ	AX
	MOVQ	AX, errno+16(FP)
	MOVQ	$0, tid+8(FP)
	RET

// cloneChildExit 以纯寄存器操作执行 SYS_EXIT（子进程夹具入口）。
TEXT ·cloneChildExit(SB),NOSPLIT,$0-0
	MOVL	$60, AX // SYS_EXIT (x86_64)
	XORL	DI, DI
	SYSCALL
	UD2 // 不可达安全网
