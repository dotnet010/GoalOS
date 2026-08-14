//go:build linux

// rawclone_linux.go — 契约测试夹具（clone_flags_contract_test.go）专用汇编入口声明。
//
// 注意：本文件不是 seccomp 过滤实现代码，是测试夹具的一部分（LP-16 夹具落点）。
// rawCloneThread / cloneChildExit 仅供 *contract_test.go 引用，生产代码不调用；
// 二者是 Go runtime 内无法安全表达的裸 clone 语义（子进程路径不触碰调用栈、
// 不执行任何 Go 代码——CLONE_VM/CLONE_THREAD 共享内存语义下的唯一安全形态）。
//
// 实现位于 rawclone_linux_amd64.s / rawclone_linux_arm64.s。
package seccomp

// rawCloneThread 以裸 clone(flags, 0, 0, 0, 0) 创建子进程/线程。
// 父进程路径返回子进程 tid；子进程路径直接跳入 cloneChildExit（SYS_EXIT），
// 永不返回——因此调用方无需（也不能）区分自身是父还是子。
func rawCloneThread(flags uintptr) (tid uintptr, errno uintptr)

// cloneChildExit 是 rawCloneThread 的子进程落点——以裸 SYS_EXIT 终止当前
// 线程，不执行任何 Go/栈操作。仅由 rawclone_linux_*.s 跳转，永不从 Go 代码调用。
// （声明为满足 go vet asmdecl 对汇编符号的 Go 声明校验。）
func cloneChildExit()
