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
#         待审议规范/（草稿）、顾问报告存档*.md（R-1658 原文存档——引用禁语原文=存档
#         合法内容，如顾问原文引用「Windows 已达真 I3」禁语恰为 R-1656 来源）
#   正文扫描: 跳过 frontmatter+修改记录区域（修改记录可合法描述"清除某旧语义"，
#             复用 check-resolution-propagation.sh 的正文提取契约）
#   兼容: repo-only 模式（GitHub Actions 无 开发文档）→ 显式降级跳过 exit 0
#
# 历史存证豁免（R-1695——PM 裁定「工具服务于工程，严禁为迎合静态检查修改历史存证」）:
#   豁免族（路径白名单，见 IGNORE_GLOBS）:
#     ① docs/stub/**          ← 本布局=开发文档/stub追踪清单.md（stub 归档索引——历史行
#                               逐字封存「某语义已于某会议废弃」，机械命中=语义冻结的假阳性）
#     ② docs/resolutions/**   ← 本布局=resolutions.yaml（决议注册表；不经本脚本扫描，
#                               路径模式登记=防未来扩围）
#     ③ red-evidence/**       ← 本布局=scripts/red-evidence/（先红/转绿存证 R-1632；
#                               同理不经本脚本扫描，登记同上）
#   边界（诚实标注）: 豁免=文件级——豁免族内**新增的活动正文**亦不机检；该族的档案真实性
#   由人工评审承载（档案体裁≠活动规范正文）。豁免非静默：逐文件输出 [skip] 行。
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
# 最后更新: 2026-09-10（R-1695——历史存证豁免白名单 IGNORE_GLOBS+is_ignored：stub 归档索引/
#                       red-evidence 存证/resolutions 注册表逐字封存，豁免非静默；此前：
#                       2026-08-13（会议 #196 R-1114 L 命名族检测扩展 + 命中输出循环 [ -n ] 守卫；
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
    "Windows[^\n]*(真 ?I3|已达 ?I3|双面齐备)"
)
# R-1656（会议 #262——顾问复审⑤采纳加牙）: Windows+真I3/已达I3/双面齐备 组合禁称——
# 06 §1.3 修订落稿前，任何文档不得声称 Windows 已达真 I3（T1-WinAC 无 seccomp
# 等价物——R-1652 v2）。阶梯语义收口=06 §1.3 修订任务（届时本模式随修订移除）。

# ─── 历史存证豁免路径白名单（R-1695——PM 裁定：白名单而非改写历史；bash 3.2 兼容=case 匹配）───
# 匹配对象=文件路径（find 输出形态：DOC_DIR 相对或绝对，两形态同匹）+ 基名。
readonly IGNORE_GLOBS=(
    "*/stub追踪清单.md"    # docs/stub/**——stub 归档索引（历史行逐字封存）
    "*/red-evidence/*"     # red-evidence/**——先红/转绿存证（R-1632）
    "*/resolutions.yaml"   # docs/resolutions/**——决议注册表
)
is_ignored() {
    local p="$1" base="${1##*/}" g
    for g in "${IGNORE_GLOBS[@]}"; do
        # shellcheck disable=SC2254  # 模式须展开为 glob（刻意不加引号）
        case "$p" in $g) return 0 ;; esac
        case "$base" in $g) return 0 ;; esac
    done
    return 1
}

# ─── L 命名族废弃模式（R-1114：独立 L0-L5 记号——前后均为非词字符或行首/行尾）───
readonly L_PATTERN='(^|[^[:alnum:]_])L[0-5]([^[:alnum:]_]|$)'

# ─── Watcher 废弃模式（R-1125：审批交互唯一呈现=CLI，弹窗范式已废弃）───
readonly WATCHER_PATTERN='(^|[^[:alnum:]_])Watcher([^[:alnum:]_]|$)'

# ─── Dashboard 废弃模式（R-1122/R-1123/R-1333：CLI 唯一软件入口，Web UI/Dashboard 已废弃）───
readonly DASHBOARD_PATTERN='(^|[^[:alnum:]_])Dashboard([^[:alnum:]_]|$)'

# ─── 弹窗废弃模式（R-1125/R-1333：审批交互唯一呈现=CLI，弹窗范式已废弃）───
readonly POPUP_PATTERN='弹窗'

# ─── 模糊词警示模式族（R-1516——会议 #235，C 顾问 Zero Interpretation 收编）───
# 警示档：命中只警告不计 FAIL（exit 0 不受影响）；W1 末评估误报率后决定是否升硬闸。
# 合法运营语义（如"必要时禁后端"类处置指令）应精确化改写，而非依赖白名单。
readonly VAGUE_PATTERN='尽可能|原则上|必要时|适当处理|根据需要|合理重试|视情况而定'

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
  [MUST] 历史存证豁免（R-1695 路径白名单）：stub 归档索引（stub追踪清单.md）/
         red-evidence 存证 / resolutions 注册表——档案体裁逐字封存，不因机检改写；
         豁免非静默（逐文件 [skip] 行）
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

# ─── 正文起始行（R-1582/S-245-01 契约修复——会议 #245：旧契约「第二个 --- 之后」对前置
# 内容多的文档漏扫（10:68 Dashboard 残留实证）。新契约：
#   ① 有「## 修改记录」节：正文起点=修改记录表结束后首个 --- 的下一行；
#      表后无 --- 时（如 12 测试清单全文无 ---）=表结束后第一个非表格行；
#   ② 无「## 修改记录」节（如 stub 追踪清单/v0.2.1 计划）：前 30 行内首个 --- 的下一行；
#   ③ 兜底=1（全扫，宁误扫不漏扫）。
# 与 check-resolution-propagation.sh 的 extract_body_start() 同算法（同名异体保持）───
body_start() {
    local fp="$1"
    awk '
        /^```/ { in_fence = !in_fence; next }
        done { next }
        !in_fence && /^## 修改记录/ { in_mod = 1; next }
        !in_fence && in_mod && /^---$/ && in_table { print NR + 1; done = 1; next }
        !in_fence && in_mod && in_table && !/^[|]/ && !table_done { table_done = 1; body_line = NR; next }
        !in_fence && in_mod && /^[|]/ { in_table = 1; next }
        !in_fence && !in_mod && /^---$/ && NR <= 30 && first_sep == 0 { first_sep = NR; next }
        END {
            if (done) exit
            if (in_mod && table_done) { print body_line; exit }
            if (first_sep) { print first_sep + 1; exit }
            print 1
        }
    ' "$fp"
}

# ─── 构建 grep -E 模式 ───
PATTERN=""
for p in "${DEPRECATED_PATTERNS[@]}"; do
    [ -z "$PATTERN" ] && PATTERN="$p" || PATTERN="$PATTERN|$p"
done

# ─── 收集检查目标文件（顶层规范文档 + 活动开发计划文档（R-1582/S-245-01 扩围——
# 开发计划/v*/ 纳入语义冻结机检；归档版本 v0.2.*/v0.3.0* 排除，沿用 check-doc-version.sh
# ARCHIVE_PATTERNS 先例）；排除历史/备份/草稿；bash 3.2 兼容：while read 替代 mapfile）───
DOC_FILES=()
while IFS= read -r fp; do
    DOC_FILES+=("$fp")
done < <(
    find "$DOC_DIR" -maxdepth 1 -name '*.md' -type f \
        ! -name '*.bak.md' \
        ! -name '会议纪要.md' \
        ! -name '顾问报告存档*.md' \
        -print 2>/dev/null | sort
)

# 开发计划/v*/ 活动计划文档（归档版本排除）
if [ -d "$DOC_DIR/开发计划" ]; then
    while IFS= read -r fp; do
        DOC_FILES+=("$fp")
    done < <(
        find "$DOC_DIR/开发计划" \
            \( -path '*/开发计划/v0.2.*' -o -path '*/开发计划/v0.3.0*' \) -prune -o \
            -name '*.md' -type f \
            ! -name '*.bak.md' \
            -print 2>/dev/null | sort
    )
fi

FAILED=0
CHECKED=0
IGNORED=0

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

    # 历史存证豁免（R-1695）——豁免非静默：逐文件明示
    if is_ignored "$fp"; then
        IGNORED=$((IGNORED + 1))
        echo -e "  ${YELLOW}[skip]${NC} $rel——历史存证豁免（R-1695 白名单：归档索引/存证/注册表）"
        continue
    fi

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

# ─── 模糊词警示段（R-1516——警示档：只警告不计 FAIL）───
VAGUE_HITS=""
for fp in "${DOC_FILES[@]}"; do
    [ -f "$fp" ] || continue
    rel="${fp#"$DOC_DIR"/}"
    is_ignored "$fp" && continue   # 历史存证豁免（R-1695）——警示段同免
    start=$(body_start "$fp")
    vhits=$(tail -n +"$start" "$fp" | grep -nE "$VAGUE_PATTERN" 2>/dev/null || true)
    if [ -n "$vhits" ]; then
        VAGUE_HITS="${VAGUE_HITS}${vhits}"$'\n'
        while IFS= read -r line; do
            lineno="${line%%:*}"
            abs=$((start + lineno - 1))
            echo -e "  ${YELLOW}[WARN]${NC} $rel:$abs [模糊词警示 R-1516]: ${line#*:}"
        done <<< "$vhits"
    fi
done
[ -n "$VAGUE_HITS" ] && echo -e "  ${YELLOW}模糊词警示（R-1516）——仅警告不计失败；处置=精确化改写（非白名单）${NC}"

echo "── 检查完成：${GREEN}$CHECKED 通过${NC} / ${RED}$FAILED 失败${NC} / ${YELLOW}$IGNORED 豁免${NC} ──"

if [ "$FAILED" -gt 0 ]; then
    echo "[ERROR] 活动规范文档正文存在废弃语义/L 命名族残留——旧语义复活风险。请按 Contract Authority 变更流程替换为新语义（R-1099 Semantic Freeze / R-1114 命名族）。" >&2
    exit 1
fi

echo "=== DEPRECATED CHECK ALL GREEN ==="
exit 0
