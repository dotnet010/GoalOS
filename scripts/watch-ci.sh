#!/usr/bin/env bash
# watch-ci.sh——CI 主动盯梢（R-1671——Jobs 2026-09-06 指令落地）：
# 「等待 CI 错误推送是被动行为」——推送后必须主动轮询（每 120 秒）到全部
# workflow 终态；本地闸口（make ci 手工等效）与 GitHub CI 双面都绿才算完。
# 事故史：windows-daily 连红 4 次无人察觉（会议 #270 观测盲区）。
#
# 用法: bash scripts/watch-ci.sh [sha]     # 缺省=当前 HEAD；短 SHA 自动解析为全 SHA
# 退出码: 0=全部 success；1=任一 failure/cancelled/timed_out 或超时（45 分钟上限）。
# 凭据: 本地走 git credential fill（token 不打印不落盘）；CI 内用 GITHUB_TOKEN 环境变量。
set -u

SHA="${1:-HEAD}"
# head_sha 过滤需全 40 位 SHA（短 SHA=零命中空转——2026-09-06 实测教训）
if [ "${#SHA}" -lt 40 ]; then
	SHA=$(git rev-parse "$SHA") || { echo "WATCH-CI FATAL: SHA 解析失败" >&2; exit 1; }
fi
REPO="dotnet010/GoalOS"
DEADLINE=$(( $(date +%s) + 2700 ))  # 45 分钟上限（windows-daily timeout 15min 冗余）

if [ -n "${GITHUB_TOKEN:-}" ]; then
	TOKEN="$GITHUB_TOKEN"
else
	TOKEN=$(printf "protocol=https\nhost=github.com\n" | git credential fill 2>/dev/null | sed -n 's/^password=//p')
fi
if [ -z "$TOKEN" ]; then
	echo "WATCH-CI FATAL: 无 GitHub 凭据（git credential 或 GITHUB_TOKEN）" >&2
	exit 1
fi

api() {
	curl -sf -H "Authorization: Bearer $TOKEN" -H "Accept: application/vnd.github+json" "$1"
}

echo "watch-ci: 盯梢 SHA=$SHA（每 120s 轮询，45 分钟上限）"
poll=0
while true; do
	poll=$((poll + 1))
	resp=$(api "https://api.github.com/repos/$REPO/actions/runs?head_sha=$SHA&per_page=20") || {
		echo "watch-ci: API 查询失败（网络/凭据）——120s 后重试"
		sleep 120
		continue
	}
	# 逐 run 抽取 status/conclusion（runs 按创建时间倒序——同一 SHA 可能多 workflow）
	summary=$(echo "$resp" | grep -oE '"status": *"[a-z_]+"|"conclusion": *("[a-z_]+"|null)|"name": *"[^"]+"' | head -60)
	total=$(echo "$resp" | grep -c '"status":')
	incomplete=$(echo "$resp" | grep -c '"status": *"\(queued\|in_progress\|waiting\|pending\|requested\)"')
	echo "--- 第 $poll 次轮询 $(date +%H:%M:%S)：runs=$total 未完=$incomplete"
	if [ "$total" -eq 0 ]; then
		if [ "$(date +%s)" -gt "$DEADLINE" ]; then
			echo "WATCH-CI FATAL: 该 SHA 无任何 workflow run（45 分钟等待耗尽）——推送未触发 CI？"
			exit 1
		fi
		sleep 120
		continue
	fi
	if [ "$incomplete" -gt 0 ]; then
		if [ "$(date +%s)" -gt "$DEADLINE" ]; then
			echo "WATCH-CI FATAL: 45 分钟上限耗尽仍有 $incomplete 个 run 未完结"
			echo "$summary"
			exit 1
		fi
		sleep 120
		continue
	fi
	# 全部完结——先打印各 run 结论摘要（可观测），再裁决（ERE 语法：分组/交替
	# 不转义——2026-09-07 实机实证事故：ERE 下 \( \| =字面量=失败检测面静默死，
	# Build failure 被误报 GREEN）
	echo "$summary"
	if echo "$resp" | grep -qE '"conclusion": *"(failure|cancelled|timed_out|action_required|startup_failure|stale)"'; then
		echo "WATCH-CI RED: 存在非 success 结论 run——立即归因修复（不许留红过夜）"
		exit 1
	fi
	echo "WATCH-CI GREEN: 全部 run success"
	exit 0
done
