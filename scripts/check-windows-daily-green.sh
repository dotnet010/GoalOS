#!/usr/bin/env bash
# check-windows-daily-green.sh——发版硬卡点（R-1672——Jobs 2026-09-06 指令：
# 「windows-daily 必须全部亮绿」=不可逾越的发版卡点）。
#
# 语义：目标 SHA 的最近一次 windows-daily run 必须 conclusion=success。
#   - 无 run=fail-closed（tag 指向的提交必须先经 main 推送触发 windows-daily）；
#   - 未完结=每 120s 轮询等待（与 R-1671 主动盯梢同节奏），上限 30 分钟；
#   - failure/cancelled/任何非 success=发布中止。
# 运行面：docker-publish.yml 前置闸（GH ubuntu runner 有 curl）；凭据=GITHUB_TOKEN。
set -u

SHA="${1:?usage: check-windows-daily-green.sh <sha>}"
REPO="${GITHUB_REPOSITORY:?need GITHUB_REPOSITORY env}"
TOKEN="${GITHUB_TOKEN:?need GITHUB_TOKEN env}"
DEADLINE=$(( $(date +%s) + 1800 ))

while true; do
	resp=$(curl -sf -H "Authorization: Bearer $TOKEN" -H "Accept: application/vnd.github+json" \
		"https://api.github.com/repos/$REPO/actions/workflows/windows-daily.yml/runs?head_sha=$SHA&per_page=1") || {
		echo "::error::windows-daily 闸口：API 查询失败（fail-closed——不发版）"
		exit 1
	}
	if echo "$resp" | grep -q '"total_count": 0'; then
		echo "::error::windows-daily 闸口：SHA=$SHA 无 windows-daily run（fail-closed——先推 main 触发）"
		exit 1
	fi
	status=$(echo "$resp" | grep -o '"status": *"[a-z_]*"' | head -1 | cut -d'"' -f4)
	conclusion=$(echo "$resp" | grep -o '"conclusion": *"[a-z_]*"' | head -1 | cut -d'"' -f4)
	echo "windows-daily 闸口：status=${status:-unknown} conclusion=${conclusion:-none}（$(date +%H:%M:%S)）"
	if [ "$status" != "completed" ]; then
		if [ "$(date +%s)" -gt "$DEADLINE" ]; then
			echo "::error::windows-daily 闸口：30 分钟等待耗尽 run 未完结（fail-closed）"
			exit 1
		fi
		sleep 120
		continue
	fi
	if [ "$conclusion" != "success" ]; then
		echo "::error::windows-daily 闸口：conclusion=${conclusion:-null} 非 success——R-1672 硬卡点，发布中止"
		exit 1
	fi
	echo "windows-daily 闸口 GREEN：SHA=$SHA 全绿——放行发布"
	exit 0
done
