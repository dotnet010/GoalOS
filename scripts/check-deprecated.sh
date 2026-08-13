#!/bin/bash
# =============================================================================
# GoalOS 废弃语义残留检测 — CI 自动化（Semantic Freeze 机检）
# =============================================================================
#
# 目的:
#   确保已废弃的旧语义不会在活动规范文档正文中残留/复活——旧语义靠人脑记忆
#   淘汰必然残留（会议 #195 事实核查：05:1112 活动正文残留"物理文件不撤销"
#   旧 rollback 语义、05:4329 残留"零值合法"放行语义）。
#
# 设计依据:
#   会议 #195 R-1099: Semantic Freeze——废弃语义 [DEPRECATED] 标注+CI 残留检测
#   会议 #195 R-1095/R-1106: stop→Failed 伪装语义/零值合法语义清除
#   会议 #196 R-1114: L 命名族废弃——L0-L5 前缀四家族拆分（风险→R0-R5/
#                     隔离→I0-I5/完整性防线→防线一/二/三/测试层级→金字塔第 N 层）
#
# 检查范围:
#   开发文档/ 顶层规范文档 *.md（01~09/GLOSSARY/模块实现指南/规范类）
#   排除: 会议纪要.md（历史记录可合法引用旧语义）、*.bak.md、开发计划/（过程文档）、
#         待审议规范/（草稿）
#   正文扫描: 跳过 frontmatter+修改记录区域（修改记录可合法描述"清除某旧语义"，
#             复用 check-resolution-propagation.sh 的正文提取契约）
#   兼容: repo-only 模式（GitHub Actions 无 开发文档）→ 显式降级跳过 exit 0
#
# 废弃模式登记（新增废弃语义时在此追加）:
#   [DEPRECATED] 物理文件不撤销       — C-11 旧 rollback 语义（R-1099，会议 #195）
#   [DEPRECATED] 零值合法             — K-C1/D-6 零值=Level 0 放行语义（R-1106）
#   [DEPRECATED] Failed(user_stopped) — C-4 stop→Failed 伪装迁移（R-1095）
#
# 命名族废弃登记（R-1114，会议 #196）:
#   L0-L5 独立记号（L 前后均为非词字符或行首/行尾）——旧风险/隔离共用前缀。
#   边界条件避免误伤 LLM/CLI/TLS(1.2)/HTML/SQL/URL/XML/LXC 等长词。
#
# 输出:
#   stdout — 逐文件结果；命中时输出 文件:行号:内容
#   exit 0 — 无废弃语义残留（或 repo-only 降级跳过）
#   exit 1 — 存在废弃语义残留
#   exit 2 — 脚本自身错误
#
# 使用:
#   bash scripts/check-deprecated.sh
#   bash scripts/check-deprecated.sh --help
#
# 维护者: GoalOS 架构团队
# 最后更新: 2026-08-13（会议 #196 R-1114 L 命名族检测扩展 + 命中输出循环 [ -n ] 守卫）
# =============================================================================

set -euo pipefail

readonly SCRIPT_NAME="$(basename "$0")"
readonly SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
readonly REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# ─── 路径自适应（R-1052 三布局，与 check-resolution-propagation.sh 同构）───
if [ -d "$SCRIPT_DIR/../../开发文档" ]; then
    DOC_DIR="$SCRIPT_DIR/../../开发文档"
    REPO_ONLY=false
elif [ -f "$REPO_DIR/resolutions.yaml" ]; then
    DOC_DIR=""
    REPO_ONLY=true
else
    DOC_DIR="开发文档"
    REPO_ONLY=false
fi
readonly DOC_DIR REPO_ONLY

# ─── 废弃模式登记表（新增废弃语义时在此追加一行）───
readonly DEPRECATED_PATTERNS=(
    "物理文件不撤销"
    "零值合法"
    "Failed(user_stopped)"
)

# ─── L 命名族废弃模式（R-1114：独立 L0-L5 记号——前后均为非词字符或行首/行尾）───
readonly L_PATTERN='(^|[^[:alnum:]_])L[0-5]([^[:alnum:]_]|$)'

# 颜色（仅终端输出；CI 重定向时自动禁用）
if [ -t 1 ]; then
    readonly RED='\033[0;31m'; readonly GREEN='\033[0;32m'
    readonly YELLOW='\033[1;33m'; readonly BOLD='\033[1m'; readonly NC='\033[0m'
else
    readonly RED='' GREEN='' YELLOW='' BOLD='' NC=''
fi

usage() {
    cat <<'EOF'
GoalOS 废弃语义残留检测 — CI 自动化（Semantic Freeze 机检）

用法:
  bash scripts/check-deprecated.sh [--help]

规则:
  [MUST] 活动规范文档正文中不得出现废弃模式（登记表见脚本头部）
  [MUST] 活动规范文档正文中不得出现独立 L0-L5 记号（R-1114 命名族废弃——
         风险→R0-R5/隔离→I0-I5；边界条件防误伤 LLM/CLI/TLS1.2 等长词）
  [MUST] 仅扫描正文——跳过 frontmatter+修改记录区域
  [MUST] 排除 会议纪要.md（历史记录）/ *.bak.md / 开发计划 / 待审议规范
  [MUST] repo-only 模式 → 显式降级跳过 exit 0

依据: 会议 #195 R-1099（Semantic Freeze）+ 会议 #196 R-1114（L 命名族废弃）
EOF
}

if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
    usage
    exit 0
fi

# repo-only 降级
if [ "$REPO_ONLY" = true ]; then
    echo "[skip] repo-only 模式——无 开发文档 目录，废弃语义检测跳过（R-1052 显式降级）"
    exit 0
fi

# ─── 正文起始行（复用 check-resolution-propagation.sh 契约：跳过 frontmatter+修改记录）───
body_start() {
    local fp="$1" start
    start=$(awk '/^---$/{n++; if(n>=2){print NR; exit}}' "$fp" 2>/dev/null)
    if [ -z "$start" ]; then
        start=10
    fi
    echo $((start + 1))
}

# ─── 构建 grep -E 模式 ───
PATTERN=""
for p in "${DEPRECATED_PATTERNS[@]}"; do
    [ -z "$PATTERN" ] && PATTERN="$p" || PATTERN="$PATTERN|$p"
done

# ─── 收集检查目标文件（顶层规范文档，排除历史/备份/草稿；bash 3.2 兼容：while read 替代 mapfile）───
DOC_FILES=()
while IFS= read -r fp; do
    DOC_FILES+=("$fp")
done < <(
    find "$DOC_DIR" -maxdepth 1 -name '*.md' -type f \
        ! -name '*.bak.md' \
        ! -name '会议纪要.md' \
        -print 2>/dev/null | sort
)

FAILED=0
CHECKED=0

echo "── ${BOLD}废弃语义残留检测${NC}（Semantic Freeze ${#DEPRECATED_PATTERNS[@]} 模式 + L 命名族 R-1114）──"

for fp in "${DOC_FILES[@]}"; do
    [ -f "$fp" ] || continue
    rel="${fp#"$DOC_DIR"/}"
    start=$(body_start "$fp")

    # 正文中命中废弃模式 → FAIL（修改记录区域已跳过）
    hits=$(tail -n +"$start" "$fp" | grep -nE "$PATTERN" 2>/dev/null || true)
    lhits=$(tail -n +"$start" "$fp" | grep -nE "$L_PATTERN" 2>/dev/null || true)

    if [ -n "$hits" ] || [ -n "$lhits" ]; then
        FAILED=$((FAILED + 1))
        echo -e "  ${RED}[FAIL]${NC} $rel 正文残留废弃语义:"
        # 注意: <<< "" 会喂入一个空行给 while read（幻影命中），必须用 [ -n ] 守卫
        if [ -n "$hits" ]; then
            while IFS= read -r line; do
                lineno="${line%%:*}"
                abs=$((start + lineno - 1))
                echo -e "    ${RED}$rel:$abs${NC} [废弃语义]: ${line#*:}"
            done <<< "$hits"
        fi
        if [ -n "$lhits" ]; then
            while IFS= read -r line; do
                lineno="${line%%:*}"
                abs=$((start + lineno - 1))
                echo -e "    ${RED}$rel:$abs${NC} [L 命名族 R-1114]: ${line#*:}"
            done <<< "$lhits"
        fi
    else
        CHECKED=$((CHECKED + 1))
        echo -e "  ${GREEN}[OK]${NC} $rel"
    fi
done

echo "── 检查完成：${GREEN}$CHECKED 通过${NC} / ${RED}$FAILED 失败${NC} ──"

if [ "$FAILED" -gt 0 ]; then
    echo "[ERROR] 活动规范文档正文存在废弃语义/L 命名族残留——旧语义复活风险。请按 Contract Authority 变更流程替换为新语义（R-1099 Semantic Freeze / R-1114 命名族）。" >&2
    exit 1
fi

echo "=== DEPRECATED CHECK ALL GREEN ==="
exit 0
