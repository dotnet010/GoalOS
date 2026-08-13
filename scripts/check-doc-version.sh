#!/bin/bash
# =============================================================================
# GoalOS 文档版本头一致性检查 — CI 自动化
# =============================================================================
#
# 目的:
#   确保每个文档头部声明的版本（**版本:** vX.Y）等于其修改记录表第一行
#   （即最新一条变更）的版本——版本头滞后=旧版本漂移（会议 #195 事实核查：
#   01 PRD 头部 v6.1 vs 修改记录最新 v6.4，滞后三版；且 05软件架构文档.bak.md
#   备份文件混入文档目录）。
#
# 设计依据:
#   会议 #195 R-1099: 唯一 Contract Authority + Semantic Freeze——
#   版本头一致性 CI 机检（check-doc-version.sh）
#
# 检查范围:
#   开发文档/ 顶层 *.md + 开发计划/ 递归 *.md
#   排除: *.bak.md（备份文件不是活动文档）、无 **版本:** 头部的文件（跳过并报告）、
#         开发计划/v0.2.* 历史版本计划目录（归档=历史记录，与 会议纪要/备份 同范畴
#         ——历史记录可合法保留旧状态，不追溯修订；新版本计划发布时若旧目录需归档，
#         将归档目录追加进 ARCHIVE_PATTERNS）
#   兼容: repo-only 模式（GitHub Actions 无 开发文档）→ 显式降级跳过 exit 0
#
# 输出:
#   stdout — 逐文件结果: [✅/⚠️跳过/❌] 文件 → 头部版本 vs 修改记录最新版本
#   exit 0 — 全部一致（或 repo-only 降级跳过）
#   exit 1 — 存在版本头与修改记录不一致的文件
#   exit 2 — 脚本自身错误
#
# 使用:
#   bash scripts/check-doc-version.sh
#   bash scripts/check-doc-version.sh --help
#
# 维护者: GoalOS 架构团队
# 最后更新: 2026-08-13（会议 #195 R-1099 首版）
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

# 颜色（仅终端输出；CI 重定向时自动禁用）
if [ -t 1 ]; then
    readonly RED='\033[0;31m'; readonly GREEN='\033[0;32m'
    readonly YELLOW='\033[1;33m'; readonly BOLD='\033[1m'; readonly NC='\033[0m'
else
    readonly RED='' GREEN='' YELLOW='' BOLD='' NC=''
fi

usage() {
    cat <<'EOF'
GoalOS 文档版本头一致性检查 — CI 自动化

用法:
  bash scripts/check-doc-version.sh [--help]

规则:
  [MUST] 每个含 **版本:** 头部的文档，头部版本必须等于修改记录表第一行的版本
  [MUST] 含 **版本:** 头部但无修改记录表 → FAIL（版本无法追溯）
  [MUST] 排除 *.bak.md 备份文件（非活动文档）
  [MUST] repo-only 模式（无 开发文档）→ 显式降级跳过 exit 0

依据: 会议 #195 R-1099（唯一 Contract Authority+Semantic Freeze）
EOF
}

if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
    usage
    exit 0
fi

# repo-only 降级（GitHub Actions 仓库布局无 开发文档）
if [ "$REPO_ONLY" = true ]; then
    echo "[skip] repo-only 模式——无 开发文档 目录，版本头一致性检查跳过（R-1052 显式降级）"
    exit 0
fi

# ─── 提取头部版本: **版本:** vX.Y（容忍 "vX.Y（注释）" 形式的头部标注）───
header_version() {
    awk -F'\\*\\*版本:\\*\\*' '
        /^\*\*版本:\*\*/ {
            # 取 **版本:** 之后的第一个非空 token，剥离尾部（…）/(…) 标注
            rest=$2; sub(/^[[:space:]]*/, "", rest)
            split(rest, t, /[[:space:]]/)
            tok=t[1]; sub(/[（(].*$/, "", tok)
            print tok; exit
        }
    ' "$1"
}

# ─── 提取修改记录最新版本: ## 修改记录 之后第一条数据行（| vX.Y |）───
changelog_latest_version() {
    awk '
        /^## 修改记录/ { in_table=1; next }
        in_table && /^\| v[0-9]/ {
            # 数据行形如 | v10.1 | 2026-08-13 | ... |
            line=$0
            split(line, f, "|")
            gsub(/[[:space:]]/, "", f[2])
            print f[2]; exit
        }
    ' "$1"
}

# ─── 收集检查目标文件（bash 3.2 兼容：while read 替代 mapfile）───
# 历史版本计划目录=归档（v0.2.*）——历史记录合法保留旧状态，不追溯修订（R-1099）
ARCHIVE_PATTERNS='*/开发计划/v0.2.*'
DOC_FILES=()
while IFS= read -r fp; do
    DOC_FILES+=("$fp")
done < <(
    find "$DOC_DIR" -maxdepth 1 -name '*.md' -type f \
        ! -name '*.bak.md' \
        -print 2>/dev/null | sort
)
while IFS= read -r fp; do
    DOC_FILES+=("$fp")
done < <(
    find "$DOC_DIR/开发计划" \( -path "$ARCHIVE_PATTERNS" -prune \) -o \
        -name '*.md' -type f \
        ! -name '*.bak.md' \
        -print 2>/dev/null | sort
)

FAILED=0
CHECKED=0
SKIPPED=0

echo "── ${BOLD}文档版本头一致性检查${NC}（头部版本 = 修改记录最新版本）──"

for fp in "${DOC_FILES[@]}"; do
    [ -f "$fp" ] || continue
    rel="${fp#"$DOC_DIR"/}"

    hver=$(header_version "$fp")
    if [ -z "$hver" ]; then
        # 无 **版本:** 头部 → 不在检查契约内（例如标题内嵌版本的计划文件），跳过并报告
        SKIPPED=$((SKIPPED + 1))
        echo "  ${YELLOW}[skip]${NC} $rel（无 **版本:** 头部）"
        continue
    fi

    cver=$(changelog_latest_version "$fp")
    if [ -z "$cver" ]; then
        FAILED=$((FAILED + 1))
        echo -e "  ${RED}[FAIL]${NC} $rel → 头部 $hver，但修改记录表缺失（版本无法追溯，R-1099）"
        continue
    fi

    if [ "$hver" = "$cver" ]; then
        CHECKED=$((CHECKED + 1))
        echo -e "  ${GREEN}[OK]${NC} $rel → $hver"
    else
        FAILED=$((FAILED + 1))
        echo -e "  ${RED}[FAIL]${NC} $rel → 头部 $hver ≠ 修改记录最新 $cver（版本头滞后=旧版本漂移，会议 #195 事实核查实证）"
    fi
done

echo "── 检查完成：${GREEN}$CHECKED 通过${NC} / ${YELLOW}$SKIPPED 跳过${NC} / ${RED}$FAILED 失败${NC} ──"

if [ "$FAILED" -gt 0 ]; then
    echo "[ERROR] 存在版本头与修改记录不一致的文档——旧版本漂移风险。请更新版本头至修改记录最新版本（R-1099）。" >&2
    exit 1
fi

echo "=== DOC VERSION CHECK ALL GREEN ==="
exit 0
