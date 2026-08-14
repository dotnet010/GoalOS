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
#   开发文档/ 顶层规范文档 *.md（01~11/00统一术语表/规范类）
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
#   [DEPRECATED] goalos status/list/log — D29 废弃命令清理（R-1164，会议 #198）
#                     顶层命令树已改为 goalos goal status/list；仅匹配 goalos 直连
#                     旧子命令（goalos goal status 等现行命令不受影响）
#   [DEPRECATED] PipelineWaiting     — D38 已废名清除改 StateWait（R-1173，会议 #198）
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
# 最后更新: 2026-08-13（会议 #196 R-1114 L 命名族检测扩展 + 命中输出循环 [ -n ] 守卫；
#                       会议 #198 R-1164——废弃命令名增补 goalos status/list/log 防回归；
#                       会议 #198 R-1173——PipelineWaiting 已废名增补防回归（改 StateWait））
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
# 注意: 含正则元字符的模式必须自含分组，避免与顶层 | 连接改变优先级
readonly DEPRECATED_PATTERNS=(
    "物理文件不撤销"
    "零值合法"
    "Failed(user_stopped)"
    "(^|[^[:alnum:]_])goalos +(status|list|log)([^[:alnum:]_]|$)"
    "(^|[^[:alnum:]_])PipelineWaiting([^[:alnum:]_]|$)"
)

# ─── L 命名族废弃模式（R-1114：独立 L0-L5 记号——前后均为非词字符或行首/行尾）───
readonly L_PATTERN='(^|[^[:alnum:]_])L[0-5]([^[:alnum:]_]|$)'

# ─── Watcher 废弃模式（R-1125：审批交互唯一呈现=CLI，弹窗范式已废弃）───
readonly WATCHER_PATTERN='(^|[^[:alnum:]_])Watcher([^[:alnum:]_]|$)'

# ─── Dashboard 废弃模式（R-1122/R-1123/R-1333：CLI 唯一软件入口，Web UI/Dashboard 已废弃）───
readonly DASHBOARD_PATTERN='(^|[^[:alnum:]_])Dashboard([^[:alnum:]_]|$)'

# ─── 弹窗废弃模式（R-1125/R-1333：审批交互唯一呈现=CLI，弹窗范式已废弃）───
readonly POPUP_PATTERN='弹窗'

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
  [MUST] 活动规范文档正文中不得出现独立 Watcher 记号（R-1125 弹窗范式废弃——
         审批交互唯一呈现=CLI）
  [MUST] 活动规范文档正文中不得出现独立 Dashboard 记号（R-1122/R-1123/R-1333——
         CLI 唯一软件入口，Web UI/Dashboard 已废弃）
  [MUST] 活动规范文档正文中不得出现"弹窗"表述（R-1125/R-1333——审批交互唯一
         呈现=CLI；代码目录扫描随 stub C-UI-01 拆除任务接线——R-1372）
  [MUST] 活动规范文档正文中不得出现废弃命令名 goalos status/list/log（R-1164
         D29 废弃命令清理——现行命令为 goalos goal status/list）
  [MUST] 活动规范文档正文中不得出现 PipelineWaiting 已废名（R-1173 D38——
         统一改 StateWait）
  [MUST] 仅扫描正文——跳过 frontmatter+修改记录区域
  [MUST] 排除 会议纪要.md（历史记录）/ *.bak.md / 开发计划 / 待审议规范
  [MUST] repo-only 模式 → 显式降级跳过 exit 0

依据: 会议 #195 R-1099（Semantic Freeze）+ 会议 #196 R-1114（L 命名族废弃）
      + 会议 #198 R-1164（废弃命令名）+ 会议 #198 R-1173（PipelineWaiting 清除）
      + 会议 #202 R-1333/R-1372（Dashboard/弹窗范式废弃——CLI 唯一入口）
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

# ─── 代码目录 Dashboard/弹窗 扫描（R-1372/C-UI-01：拆除任务后接线；仅 .go/.html）───
# 豁免规则：注释中以"已拆除 R-1372"或"已废弃"标注的历史说明；测试文件中的拆字字面量
# （"Handle"+"Dash"+"board" 拼接）天然不匹配本扫描。
CODE_DASHBOARD_HITS=""
if [ "$REPO_ONLY" = false ] && [ -d "$REPO_DIR/internal" ]; then
    while IFS= read -r cpf; do
        # 豁免：拆除验证测试文件（其断言文案必须点名被拆对象）；标注"已拆除 R-1372"的行；纯注释行
        case "$cpf" in
            *dashboard_removed_contract_test.go) continue ;;
        esac
        # 跳过标注"已拆除 R-1372"的行与纯注释说明行
        chits=$(grep -nE "$DASHBOARD_PATTERN" "$cpf" 2>/dev/null \
            | grep -v "已拆除 R-1372" \
            | grep -vE "^\s*[0-9]+:\s*//" || true)
        [ -n "$chits" ] && CODE_DASHBOARD_HITS="${CODE_DASHBOARD_HITS}${cpf}\n${chits}\n"
    done < <(find "$REPO_DIR/internal" "$REPO_DIR/cmd" -name '*.go' -o -name '*.html' 2>/dev/null)
fi

echo "── ${BOLD}废弃语义残留检测${NC}（Semantic Freeze ${#DEPRECATED_PATTERNS[@]} 模式 + L 命名族 R-1114 + Watcher R-1125 + Dashboard/弹窗 R-1333 + 代码目录 R-1372）──"

for fp in "${DOC_FILES[@]}"; do
    [ -f "$fp" ] || continue
    rel="${fp#"$DOC_DIR"/}"
    start=$(body_start "$fp")

    # 正文中命中废弃模式 → FAIL（修改记录区域已跳过）
    hits=$(tail -n +"$start" "$fp" | grep -nE "$PATTERN" 2>/dev/null || true)
    lhits=$(tail -n +"$start" "$fp" | grep -nE "$L_PATTERN" 2>/dev/null || true)
    whits=$(tail -n +"$start" "$fp" | grep -nE "$WATCHER_PATTERN" 2>/dev/null || true)
    dhits=$(tail -n +"$start" "$fp" | grep -nE "$DASHBOARD_PATTERN" 2>/dev/null || true)
    phits=$(tail -n +"$start" "$fp" | grep -nE "$POPUP_PATTERN" 2>/dev/null || true)

    if [ -n "$hits" ] || [ -n "$lhits" ] || [ -n "$whits" ] || [ -n "$dhits" ] || [ -n "$phits" ]; then
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
        if [ -n "$whits" ]; then
            while IFS= read -r line; do
                lineno="${line%%:*}"
                abs=$((start + lineno - 1))
                echo -e "    ${RED}$rel:$abs${NC} [Watcher 废弃 R-1125]: ${line#*:}"
            done <<< "$whits"
        fi
        if [ -n "$dhits" ]; then
            while IFS= read -r line; do
                lineno="${line%%:*}"
                abs=$((start + lineno - 1))
                echo -e "    ${RED}$rel:$abs${NC} [Dashboard 废弃 R-1122/R-1123/R-1333]: ${line#*:}"
            done <<< "$dhits"
        fi
        if [ -n "$phits" ]; then
            while IFS= read -r line; do
                lineno="${line%%:*}"
                abs=$((start + lineno - 1))
                echo -e "    ${RED}$rel:$abs${NC} [弹窗废弃 R-1125/R-1333]: ${line#*:}"
            done <<< "$phits"
        fi
    else
        CHECKED=$((CHECKED + 1))
        echo -e "  ${GREEN}[OK]${NC} $rel"
    fi
done

if [ -n "$CODE_DASHBOARD_HITS" ]; then
    FAILED=$((FAILED + 1))
    echo -e "  ${RED}[FAIL]${NC} 代码目录（internal/cmd）残留 Dashboard 引用（R-1372/C-UI-01）:"
    echo -e "$CODE_DASHBOARD_HITS" | grep -v '^$' | head -20
fi

echo "── 检查完成：${GREEN}$CHECKED 通过${NC} / ${RED}$FAILED 失败${NC} ──"

if [ "$FAILED" -gt 0 ]; then
    echo "[ERROR] 活动规范文档正文存在废弃语义/L 命名族残留——旧语义复活风险。请按 Contract Authority 变更流程替换为新语义（R-1099 Semantic Freeze / R-1114 命名族）。" >&2
    exit 1
fi

echo "=== DEPRECATED CHECK ALL GREEN ==="
exit 0
