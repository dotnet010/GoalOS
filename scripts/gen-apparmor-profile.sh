#!/bin/bash
# gen-apparmor-profile.sh——Ubuntu 24.04 userns 授权 profile 生成器（S-266-03——
# 会议 #267 落地序列；Flatpak/bwrap 先例形态）。
#
# 背景：Ubuntu 24.04 默认策略（AppArmor 4.0）限制非特权 userns 创建——
# kernel.apparmor_restrict_unprivileged_userns=1 时无 profile 的二进制
# unshare(CLONE_NEWUSER)=被拒（goalos-test 实机实证）。
# 形态纪律（实机排障链实证）：**必须 flags=(unconfined)+userns 规则**——
# confined 形态=逐条加规则的跑步机（proc/sys/usr/cgroup/network/sys_admin/exec
# 继承逐拦，审计日志实锤）；禁 aa-enforce 加载（剥 unconfined 语义）；
# complain 模式不满足内核 userns 检查。
#
# 用法: gen-apparmor-profile.sh <二进制绝对路径> [profile名]
#      产出=测试/工作二进制 profile（unconfined+userns）
#       gen-apparmor-profile.sh --bwrap
#      产出=bwrap 放行 profile（userns+net_admin+setpcap+sys_admin——bwrap 在
#      Ubuntu 24.04 的 unprivileged_userns 兜底 profile 下被剥能力=回环拉起
#      RTM_NEWADDR 被拒实机实锤的修正面）
# 产出: stdout=profile 文本；安装=apparmor_parser -r 加载（调用方 sudo 面）。
set -euo pipefail

if [ "${1:-}" = "--bwrap" ]; then
	cat <<'BWRAP_PROFILE'
# GoalOS bwrap 放行 profile（实机实证：Ubuntu 24.04 下 bwrap 经 unprivileged_userns
# 兜底 profile 被剥 net_admin/setpcap/sys_admin=回环拉起 RTM_NEWADDR 被拒=bwrap 死）。
# 本 profile=unconfined+所需能力+userns——bwrap 自身=安全边界，profile 只为它开口。
abi <abi/4.0>,
include <tunables/global>

profile goalos-bwrap /usr/bin/bwrap flags=(unconfined) {
  userns,
  capability net_admin,
  capability setpcap,
  capability sys_admin,
}
BWRAP_PROFILE
	exit 0
fi

BINPATH="${1:?usage: gen-apparmor-profile.sh [--bwrap]|<二进制绝对路径> [profile名]}"
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
# userns_create LSM 钩；其余一切不施加限制（边界=二进制自身沙箱机制——2026-09-08
# 起模式 A=bwrap 承载，agentbox 族已全量移除）。
# 纪律：禁 aa-enforce 加载（会剥 unconfined 语义）；complain 模式不满足内核检查。
abi <abi/4.0>,
include <tunables/global>

profile $PNAME "$BINPATH" flags=(unconfined) {
  userns,
}
PROFILE
