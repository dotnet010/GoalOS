#!/bin/bash
# gen-apparmor-profile.sh——模式 A 正证用 AppArmor profile 生成器（S-266-03——
# 会议 #267 落地序列；Flatpak 先例形态）。
#
# 背景：Ubuntu 24.04 默认策略（AppArmor 4.0）限制非特权 userns 创建——
# kernel.apparmor_restrict_unprivileged_userns=1 时无 profile 的二进制
# unshare(CLONE_NEWUSER)=EPERM（goalos-test 实机实证）。
# 本生成器产出「指定二进制路径=授权 userns」的 profile——不翻全局 sysctl
# （不动系统默认面），只给具名二进制开口。
#
# 用法: gen-apparmor-profile.sh <二进制绝对路径> [profile名]
# 产出: stdout=profile 文本；安装=apparmor_parser -r 加载（调用方 sudo 面）。
set -euo pipefail

BINPATH="${1:?usage: gen-apparmor-profile.sh <二进制绝对路径> [profile名]}"
PNAME="${2:-goalos-mode-a-probe}"

if [ ! -e "$BINPATH" ]; then
    echo "gen-apparmor-profile: 二进制不存在: $BINPATH" >&2
    exit 2
fi
# 路径规范化（AppArmor 按真实路径匹配——符号链接解析）
BINPATH=$(readlink -f "$BINPATH")

cat <<PROFILE
# GoalOS 模式 A 正证 profile（S-266-03——gen-apparmor-profile.sh 生成，勿手编）
# 语义（Ubuntu 24.04 userns 限制官方形态——bwrap 先例）：flags=(unconfined)+
# userns 规则——本 profile 唯一职责=让二进制成为「有 profile」进程过内核
# userns_create LSM 钩；其余一切不施加限制（边界=agentbox 自身 namespace 族）。
# 纪律：禁 aa-enforce 加载（会剥 unconfined 语义）；complain 模式不满足内核检查。
# 实机教训：confined 形态（逐条加文件/能力规则）=跑步机（Go runtime 读 /proc/
# /sys/agentbox 读 mountinfo/exec 继承/网络族——逐条都拦），官方形态即正确解。
abi <abi/4.0>,
include <tunables/global>

profile $PNAME "$BINPATH" flags=(unconfined) {
  userns,
}
PROFILE
