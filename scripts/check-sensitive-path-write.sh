#!/bin/bash
# =============================================================================
# check-sensitive-path-write.sh——测试夹具敏感路径写入机检（会议 #272 提案落地）
#
# 事故史（2026-09-06 实证）：WinAC Boundary②测试直接 os.WriteFile(真实
# ~/.ssh/config)——毁损用户 SSH 配置（goalos-test 登录条目丢失）。
# 教训：测试=可能跑在任何人的机器上——夹具永不写真实用户既有文件。
#
# 规则（三条件同文件同满=红——反证迭代两轮实证：单行模式漏过变量间接形态，
# 纯构造匹配误伤分类断言）：
#   ①文件构造敏感目录（".ssh"/".aws"/".gnupg" 字面量）
#   ②文件出现敏感文件终段字面量（"config"/"id_*"/"credentials"/".gitconfig"/
#     ".netrc"/"known_hosts"）且该行无豁免标记（goalos- 夹具名 或 safefixture: 注释）
#   ③文件含写入动词（os.WriteFile/Create/MkdirAll/Remove/RemoveAll）
# 纯分类/纯读取（无写入动词）=合法通过；goalos- 专用夹具名=合法新建。
#
# 检查工具面=scripts/toolcheck senswrite（2026-09-10——R-1676 同族：检查链原生
# Go 工具面；本脚本=薄封装，核心判定=Go 原生单二进制，本地/CI 同工具链同行为；
# bash 段判定逻辑已随移植退役=单实现面零漂移）。
# 退出码: 0=无违规, 1=存在违规, 2=工具面错误
# =============================================================================
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TOOLCHECK="$(mktemp)"
trap 'rm -f "$TOOLCHECK"' EXIT

if ! (cd "$SCRIPT_DIR/.." && go build -o "$TOOLCHECK" ./scripts/toolcheck/); then
    echo "❌ toolcheck 构建失败（scripts/toolcheck——go 工具链面）" >&2
    exit 2
fi

exec "$TOOLCHECK" senswrite
