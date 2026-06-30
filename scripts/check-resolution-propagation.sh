#!/bin/bash
# =============================================================================
# GoalOS 决议传播完整性检查 — CI 自动化（两层验证）
# =============================================================================
#
# 目的:
#   确保每个架构决议（R-xxx）不仅在文档的"修改记录"中被引用（层1），
#   而且在文档正文中真正落地（层2——must_exist / must_not_exist）。
#
# 设计依据:
#   会议 #81 R-481: 决议传播完整性 CI 检查——根源修复 1
#   会议 #82 R-485: resolutions.yaml 作为机器可读的单一真相来源
#   会议 #84: 增强层2——正文内容一致性基于 grep 模式匹配
#   开发规范 §1.1: 三层正确性——代码正确 + 行为正确 + 设计正确
#
# 输入:
#   $1 — resolutions.yaml 路径（默认: GoalOS/resolutions.yaml）
#
# 输出:
#   stdout — 逐条检查结果（层1 + 层2）。每行: [✅/❌] 决议 → 文件 → 详情
#   stderr — 诊断信息和错误
#   exit 0 — 全部检查通过。可进入开发
#   exit 1 — 存在未传播的决议。开发阻塞
#   exit 2 — 脚本自身错误（配置文件不存在/YAML 格式错误/依赖缺失）
#
# 行为契约:
#   [MUST]     层1: 验证每个决议的 files 列表中所有文件的修改记录引用了该决议编号
#   [MUST]     层2: 验证每项 verify 规则——must_exist 模式在正文中存在 / must_not_exist 模式在正文中不存在
#   [MUST]     层2 must_not_exist 检查只扫描正文（跳过修改记录区域）
#   [MUST]     exit code 精确反映失败数量: 0=全通过, 1=有未传播项, 2=脚本错误
#   [MUST]     每项检查输出至少包含: 决议编号、文件名、通过/失败、失败时的详情（模式/原因/匹配行）
#   [MUST]     支持 --verbose 模式输出每步执行细节
#   [MUST]     支持 --help 输出使用说明
#   [MUST_NOT] 使用 subshell 导致 FAILED 计数丢失（Bug #1: pipe→临时文件替代）
#   [MUST_NOT] 将 IFS 设置为多字符分隔符，导致字段错误拆分（Bug #2: "|||"→TAB）
#   [MUST_NOT] 对修改记录区域执行 must_not_exist 检查，导致假阳性（Bug #3: 正文提取）
#
# 使用:
#   bash scripts/check-resolution-propagation.sh                  # 默认运行
#   bash scripts/check-resolution-propagation.sh --verbose        # 详细模式
#   bash scripts/check-resolution-propagation.sh --help           # 帮助
#   bash scripts/check-resolution-propagation.sh path/to/custom.yaml
#
# 维护者: GoalOS 架构团队
# 最后更新: 2026-06-28（会议 #84 重写——正文提取/字段分隔/TAB 修复）
# =============================================================================

set -euo pipefail

# ─── 常量 ───
readonly SCRIPT_NAME="$(basename "$0")"
readonly DOC_DIR="开发文档"
readonly DEFAULT_RESOLUTIONS="GoalOS/resolutions.yaml"
readonly DELIM=$'\t'  # TAB——不会出现在 YAML 值或 grep 模式中

# 颜色（仅终端输出；CI 重定向时自动禁用）
if [ -t 1 ]; then
    readonly RED='\033[0;31m'
    readonly GREEN='\033[0;32m'
    readonly YELLOW='\033[1;33m'
    readonly BOLD='\033[1m'
    readonly NC='\033[0m'
else
    readonly RED='' GREEN='' YELLOW='' BOLD='' NC=''
fi

# ─── 状态变量 ───
VERBOSE=false
FAILED=0
CHECKS_TOTAL=0
LAYER1_PASS=0;  LAYER1_FAIL=0
LAYER2_PASS=0;  LAYER2_FAIL=0
START_TIME=$(date +%s)

# =============================================================================
# 函数: log_info / log_verbose / log_error / log_success
# 用途: 统一日志输出。verbose 消息仅 --verbose 模式显示。
# =============================================================================

log_info()  { echo -e "$*"; }
log_verbose() { [ "$VERBOSE" = true ] && echo -e "  ${YELLOW}[verbose]${NC} $*" || true; }
log_error() { echo -e "${RED}[ERROR]${NC} $*" >&2; }
log_success() { echo -e "${GREEN}[OK]${NC} $*"; }

# =============================================================================
# 函数: usage
# 用途: 输出帮助文本。任何 --help 或非法参数触发。
# =============================================================================

usage() {
    cat <<'EOF'
GoalOS 决议传播完整性检查 — CI 自动化（两层验证）

用法:
  bash scripts/check-resolution-propagation.sh [选项] [resolutions.yaml路径]

选项:
  --verbose, -v    详细模式——输出每步执行细节（文件路径解析、grep 命令、匹配内容）
  --help, -h       显示此帮助

示例:
  bash scripts/check-resolution-propagation.sh
  bash scripts/check-resolution-propagation.sh --verbose
  bash scripts/check-resolution-propagation.sh GoalOS/resolutions.yaml

两层验证:
  层1（元数据追溯）: 每个文件的"修改记录"表必须引用决议编号（R-xxx）
  层2（正文内容一致性）: 文档正文必须满足 verify 规则——must_exist / must_not_exist

退出码:
  0 — 全部通过。决议传播完整
  1 — 存在未传播的决议。必须在写代码前修复
  2 — 脚本自身错误（配置缺失/格式错误/依赖缺失）

配置:
  决议追踪配置: GoalOS/resolutions.yaml
  检查脚本:       GoalOS/scripts/check-resolution-propagation.sh
EOF
    exit 0
}

# =============================================================================
# 函数: resolve_path [filename]
# 用途: 将 resolutions.yaml 中的相对文件名解析为实际文件系统路径。
#       查找顺序: 开发文档/<f> → 开发文档/开发计划/v0.2.0/<f> → GoalOS/<f>
# 输入: $1 — 文件名（可能包含子目录前缀，如 "开发计划/v0.2.0/v0.2.0-4-开发计划.md"）
# 输出: stdout — 解析后的绝对/相对路径。如果文件不存在，输出空字符串
# =============================================================================

resolve_path() {
    local f="$1"

    # 开发计划子目录文件（v0.2.0-* 等）
    if echo "$f" | grep -qE '^开发计划/v[0-9]'; then
        echo "$DOC_DIR/$f"
        return
    fi

    # 基石文档（开发文档/ 目录下）
    case "$f" in
        01-prd产品需求文档.md|02低保真原型.md|03高保真交互原型.md|\
        04命令行界面设计规范.md|05软件架构文档.md|06安全模型文档.md|\
        07事件注册表.md|08沙箱隔离与进程通信规范.md|\
        GLOSSARY.md|架构会议规范.md|会议纪要.md|\
        stub追踪清单.md|开发就绪检查清单.md)
            echo "$DOC_DIR/$f"
            ;;
        # 开发计划子目录文件（vX.Y.Z-N-*.md 等）
        v[0-9]*开发计划.md|v[0-9]*测试计划.md|v[0-9]*预期目标与验收标准.md|v[0-9]*顾问审计意见.md)
            echo "$DOC_DIR/开发计划/v0.2.0/$f"
            ;;
        # GoalOS/ 目录下的规范文档
        测试规范.md|开发规范.md|发布规范.md|验收规范.md|用户手册.md|开发日志.md)
            echo "GoalOS/$f"
            ;;
        check-anti-cheat.sh|check-cross-verification.sh|check-doc-completeness.sh|check-schema-consistency.sh|check-plugin-protocol.sh|export-glossary-yaml.sh)
            echo "GoalOS/scripts/$f"
            ;;
        *)
            # 模糊查找——尝试多个位置
            if [ -f "$DOC_DIR/$f" ]; then
                echo "$DOC_DIR/$f"
            elif [ -f "$f" ]; then
                echo "$f"
            else
                echo ""
            fi
            ;;
    esac
}

# =============================================================================
# 函数: extract_body_start [file]
# 用途: 找到文档正文的起始行号（跳过 frontmatter + 修改记录）。
#       文档结构: frontmatter(版本/状态) → --- → 修改记录表 → --- → 正文
#       返回第二个 "---" 之后的第一行行号。
# 输入: $1 — 文档文件路径
# 输出: stdout — 正文起始行号。如果找不到第二个 "---"，返回 10（fallback）
# =============================================================================

extract_body_start() {
    local fp="$1"
    local start
    start=$(awk '/^---$/{n++; if(n>=2){print NR; exit}}' "$fp" 2>/dev/null)
    if [ -z "$start" ]; then
        start=10  # fallback: 跳过 frontmatter
    fi
    # +1 跳过 --- 行本身，从正文第一行开始
    echo $((start + 1))
}

# =============================================================================
# 函数: check_layer1 [resolutions_yaml]
# 用途: 层1——元数据追溯。验证每个决议的 files 列表中所有文件的修改记录引用了决议编号。
#       逐行读取 YAML→提取决议和 files→对每个文件执行 grep 检查。
# 输入: $1 — resolutions.yaml 路径
# 副作用: 更新 LAYER1_PASS, LAYER1_FAIL, FAILED, CHECKS_TOTAL
# 输出: stdout — 逐条检查结果
# =============================================================================

check_layer1() {
    local yaml="$1"
    local current_r="" files=""

    log_info "── ${BOLD}层1: 元数据追溯${NC}（修改记录引用决议编号）──"
    log_info "  规则: resolutions.yaml 中每个决议的 files 列表 → 每个文件的修改记录必须引用决议编号"
    log_info ""

    while IFS= read -r line; do
        # 匹配决议行: "  R-xxx:"
        if echo "$line" | grep -qE '^  R-[0-9]+:$'; then
            current_r=$(echo "$line" | grep -oE 'R-[0-9]+')
            files=""
        fi

        # 匹配 files 行: "    files: [...]"
        if [ -n "${current_r:-}" ] && echo "$line" | grep -qE '^\s+files:\s*\['; then
            files=$(echo "$line" \
                | sed 's/.*files: \[//' \
                | sed 's/\].*//' \
                | tr ',' '\n' \
                | sed "s/['\"]//g" \
                | sed 's/^ *//' \
                | sed 's/ *$//')

            local file_count=0
            for f in $files; do
                [ -z "$f" ] && continue
                file_count=$((file_count + 1))
                local fp
                fp=$(resolve_path "$f")

                if [ -z "$fp" ] || [ ! -f "$fp" ]; then
                    log_error "$current_r → $f — ${RED}文件不存在${NC}（路径: $fp）"
                    FAILED=$((FAILED + 1))
                    LAYER1_FAIL=$((LAYER1_FAIL + 1))
                    CHECKS_TOTAL=$((CHECKS_TOTAL + 1))
                    continue
                fi

                CHECKS_TOTAL=$((CHECKS_TOTAL + 1))
                log_verbose "  检查: grep -q '$current_r' '$fp'"

                if grep -q "$current_r" "$fp" 2>/dev/null; then
                    echo -e "  ${GREEN}✅${NC} $current_r → $(basename "$fp")"
                    LAYER1_PASS=$((LAYER1_PASS + 1))
                else
                    echo -e "  ${RED}❌${NC} $current_r → $(basename "$fp") — ${RED}修改记录未引用 $current_r${NC}"
                    FAILED=$((FAILED + 1))
                    LAYER1_FAIL=$((LAYER1_FAIL + 1))
                fi
            done
            log_verbose "  $current_r: 检查了 $file_count 个文件"
            current_r=""
            files=""
        fi
    done < "$yaml"

    echo ""
    echo -e "  ${BOLD}层1 结果${NC}: ${GREEN}$LAYER1_PASS 通过${NC} / ${RED}$LAYER1_FAIL 失败${NC}"
    echo ""
}

# =============================================================================
# 函数: extract_verify_rules [resolutions_yaml] → $temp_file
# 用途: 从 resolutions.yaml 中提取所有 verify 规则（must_exist / must_not_exist）。
#       使用 awk 解析 YAML——输出 TAB 分隔的标准化格式。
#       输出格式: CMD\tRESOLUTION\tPATTERN（CMD=MUST_EXIST|MUST_NOT_EXIST|IN_FILE|REASON）
# 输入: $1 — resolutions.yaml 路径
# 输出: stdout（重定向到临时文件）— TAB 分隔的 verify 规则
# =============================================================================

extract_verify_rules() {
    local yaml="$1"
    awk -v D="$DELIM" '
    /^  R-[0-9]+:/            { r=$1; gsub(/:/,"",r) }
    /^      - must_not_exist:/ { p=$0; gsub(/.*must_not_exist: "/,"",p); gsub(/".*/,"",p)
                                 print "MUST_NOT_EXIST" D r D p }
    /^      - must_exist:/     { p=$0; gsub(/.*must_exist: "/,"",p); gsub(/".*/,"",p)
                                 print "MUST_EXIST" D r D p }
    /^        in_file:/        { f=$0; gsub(/.*in_file: /,"",f)
                                 print "IN_FILE" D f }
    /^        reason:/         { rs=$0; gsub(/.*reason: "/,"",rs); gsub(/".*/,"",rs)
                                 print "REASON" D rs }
    ' "$yaml"
}

# =============================================================================
# 函数: check_layer2 [resolutions_yaml]
# 用途: 层2——正文内容一致性。验证每项 verify 规则:
#         must_exist: 正文中必须存在指定模式
#         must_not_exist: 正文中禁止存在指定模式（仅扫描正文——跳过修改记录）
#       从临时文件读取规则（避免 pipe subshell 导致变量丢失）。
# 输入: $1 — resolutions.yaml 路径
# 副作用: 更新 LAYER2_PASS, LAYER2_FAIL, FAILED, CHECKS_TOTAL
# 输出: stdout — 逐条检查结果
# =============================================================================

check_layer2() {
    local yaml="$1"
    local rules_file
    rules_file=$(mktemp)
    # shellcheck disable=SC2064
    trap "rm -f '$rules_file'" EXIT

    log_info "── ${BOLD}层2: 正文内容一致性${NC}（文档正文满足 verify 规则）──"
    log_info "  规则: must_exist 模式在正文中必须存在 / must_not_exist 模式在正文中禁止出现"
    log_info "  正文定义: 第二个 '---' 分隔符之后的内容。修改记录中的历史描述不参与检查。"
    log_info ""

    # Step 1: 提取 verify 规则到临时文件
    extract_verify_rules "$yaml" > "$rules_file"
    local rule_count
    rule_count=$(wc -l < "$rules_file")
    log_verbose "提取了 $rule_count 行 verify 规则 → $rules_file"

    # Step 2: 逐行处理规则
    local rt="" rid="" pat="" inf="" rsn=""

    while IFS="$DELIM" read -r cmd r arg; do
        case "$cmd" in
            MUST_NOT_EXIST)
                rt="must_not_exist"
                rid="$r"
                pat="$arg"
                inf=""
                log_verbose "规则: $rid $rt — 模式: '$pat'"
                ;;
            MUST_EXIST)
                rt="must_exist"
                rid="$r"
                pat="$arg"
                inf=""
                log_verbose "规则: $rid $rt — 模式: '$pat'"
                ;;
            IN_FILE)
                inf="$r"  # IN_FILE\t<filename> — 两字段格式
                log_verbose "  in_file: $inf"
                ;;
            REASON)
                rsn="$r"  # REASON\t<reason_text> — 两字段格式

                # 确定检查目标文件
                local fp
                if [ -n "$inf" ]; then
                    fp=$(resolve_path "$inf")
                else
                    # 无 in_file——从 files 列表取第一个文件
                    local files_line first_file
                    files_line=$(grep -A20 "^  $rid:" "$yaml" | grep "files:" | head -1)
                    first_file=$(echo "$files_line" \
                        | sed 's/.*files: \[//' \
                        | sed 's/\].*//' \
                        | sed "s/['\"]//g" \
                        | cut -d',' -f1 \
                        | sed 's/^ *//')
                    fp=$(resolve_path "$first_file")
                fi

                # 验证目标文件
                if [ -z "$fp" ] || [ ! -f "$fp" ]; then
                    CHECKS_TOTAL=$((CHECKS_TOTAL + 1))
                    log_error "$rid → ${inf:-$first_file} — ${RED}文件不存在${NC}"
                    FAILED=$((FAILED + 1))
                    LAYER2_FAIL=$((LAYER2_FAIL + 1))
                    continue
                fi

                CHECKS_TOTAL=$((CHECKS_TOTAL + 1))

                # 执行检查
                if [ "$rt" = "must_not_exist" ]; then
                    _check_must_not_exist "$rid" "$pat" "$fp" "$rsn"
                else
                    _check_must_exist "$rid" "$pat" "$fp" "$rsn"
                fi
                ;;
        esac
    done < "$rules_file"

    rm -f "$rules_file"
    trap - EXIT

    echo ""
    echo -e "  ${BOLD}层2 结果${NC}: ${GREEN}$LAYER2_PASS 通过${NC} / ${RED}$LAYER2_FAIL 失败${NC}"
    echo ""
}

# ─── 层2 子函数: _check_must_not_exist ───

_check_must_not_exist() {
    local rid="$1" pat="$2" fp="$3" rsn="$4"
    local body_start body_matches

    # 提取正文起始行（跳过修改记录）
    body_start=$(extract_body_start "$fp")
    log_verbose "  正文起始行: $body_start（跳过修改记录区域）"
    log_verbose "  执行: tail -n +$body_start '$fp' | grep -nE '$pat'"

    # 在正文中搜索
    body_matches=$(tail -n +"$body_start" "$fp" | grep -nE "$pat" 2>/dev/null || true)

    if [ -n "$body_matches" ]; then
        echo -e "  ${RED}❌${NC} $rid → $(basename "$fp"): ${RED}禁止模式仍存在（正文）${NC}"
        echo "     模式: $pat"
        [ -n "$rsn" ] && echo "     原因: $rsn"
        echo "     正文行号(从第${body_start}行起):"
        echo "$body_matches" | head -3 | while IFS= read -r match_line; do
            echo "       → $match_line"
        done
        FAILED=$((FAILED + 1))
        LAYER2_FAIL=$((LAYER2_FAIL + 1))
    else
        local reason_str=""
        [ -n "$rsn" ] && reason_str=" — $rsn"
        echo -e "  ${GREEN}✅${NC} $rid → $(basename "$fp"): 禁止模式已清除$reason_str"
        LAYER2_PASS=$((LAYER2_PASS + 1))
    fi
}

# ─── 层2 子函数: _check_must_exist ───

_check_must_exist() {
    local rid="$1" pat="$2" fp="$3" rsn="$4"

    log_verbose "  执行: grep -qE '$pat' '$fp'"

    if grep -qE "$pat" "$fp" 2>/dev/null; then
        local reason_str=""
        [ -n "$rsn" ] && reason_str=" — $rsn"
        echo -e "  ${GREEN}✅${NC} $rid → $(basename "$fp"): 必需内容已存在$reason_str"
        LAYER2_PASS=$((LAYER2_PASS + 1))
    else
        echo -e "  ${RED}❌${NC} $rid → $(basename "$fp"): ${RED}必需内容缺失${NC}"
        echo "     模式: $pat"
        [ -n "$rsn" ] && echo "     原因: $rsn"
        FAILED=$((FAILED + 1))
        LAYER2_FAIL=$((LAYER2_FAIL + 1))
    fi
}

# =============================================================================
# 函数: check_ghost_resolutions [resolutions_yaml]
# 用途: 检测"幽灵决议"——在文档修改记录中被引用但在 resolutions.yaml 中不存在的 R-xxx 编号。
#       验证空号区间注释与实际文档引用之间的一致性（R-544）。
#       如果文档引用了一个空号区间内的号码但没有对应注释→可能是幽灵决议。
# 输入: $1 — resolutions.yaml 路径
# 副作用: 更新 FAILED, GHOST_FAIL
# =============================================================================

check_ghost_resolutions() {
    local yaml="$1"
    local GHOST_FAIL=0

    log_info "── ${BOLD}幽灵决议检测${NC}（文档引用的 R-xxx 必须在 resolutions.yaml 中存在）──"
    log_info ""

    # Step 1: 从 resolutions.yaml 提取所有有效 R-xxx 编号
    local valid_resolutions
    valid_resolutions=$(mktemp)
    grep -oE 'R-[0-9]+' "$yaml" | grep -oE '[0-9]+' | sort -n | uniq > "$valid_resolutions"

    # Step 2: 扫描所有文档中的 R-xxx 引用（排除 resolutions.yaml 自身和 check script）
    local all_refs
    all_refs=$(mktemp)
    find "$DOC_DIR" "GoalOS" -name "*.md" -o -name "*.yaml" 2>/dev/null | \
        grep -v "resolutions.yaml" | \
        grep -v "会议纪要.md" | \
        xargs grep -ohE 'R-[0-9]+' 2>/dev/null | \
        grep -oE '[0-9]+' | sort -n | uniq > "$all_refs"

    # Step 3: 找出在文档中被引用但不在 resolutions.yaml 中的 R-xxx
    local ghost_count=0
    while IFS= read -r num; do
        if ! grep -q "^$num$" "$valid_resolutions" 2>/dev/null; then
            # R-1~R-393: 预 v0.2.0 决议——在会议纪要中，合法
            if [ "$num" -le 393 ]; then
                continue
            fi
            # R-508: 空决议（files:[]），合法
            if [ "$num" = "508" ]; then
                continue
            fi
            # 检查是否在注释的空号区间中
            if grep -q "R-$num.*空号\|$num.*空号\|~$num\|$num~\|$num.*跳过" "$yaml" 2>/dev/null; then
                log_verbose "R-$num: 空号区间内——已注释"
                continue
            fi

            # 幽灵决议！
            ghost_count=$((ghost_count + 1))
            # 找到引用此编号的文档
            local ref_files
            ref_files=$(find "$DOC_DIR" "GoalOS" -name "*.md" 2>/dev/null | \
                xargs grep -l "R-$num" 2>/dev/null | head -3 | tr '\n' ' ')
            echo -e "  ${RED}❌ R-$num${NC}: ${RED}幽灵决议${NC}——被文档引用但不在 resolutions.yaml 中"
            echo "     引用文件: $ref_files"
            FAILED=$((FAILED + 1))
            GHOST_FAIL=$((GHOST_FAIL + 1))
        fi
    done < "$all_refs"

    rm -f "$valid_resolutions" "$all_refs"

    if [ "$ghost_count" -eq 0 ]; then
        echo -e "  ${GREEN}✅${NC} 幽灵决议检测通过——所有文档引用的 R-xxx 均在 resolutions.yaml 中有定义"
    fi
    echo ""
    echo -e "  ${BOLD}幽灵检测结果${NC}: ${GREEN}$(( $(grep -cE '^[0-9]+$' "$valid_resolutions" 2>/dev/null || echo 0) - ghost_count )) 有效${NC} / ${RED}$GHOST_FAIL 幽灵${NC}"
    echo ""
}

# =============================================================================
# 函数: print_summary
# 用途: 输出最终总结——包括耗时、检查项数、通过/失败统计、修复建议。
# =============================================================================

print_summary() {
    local elapsed
    elapsed=$(($(date +%s) - START_TIME))

    echo "──────────────────────────────"
    if [ $FAILED -eq 0 ]; then
        echo -e "${GREEN}${BOLD}✅ 决议传播完整性检查通过（两层验证）${NC}"
    else
        echo -e "${RED}${BOLD}❌ $FAILED 项未通过${NC}"
    fi
    echo "  总检查项: $CHECKS_TOTAL"
    echo "  层1（修改记录引用）: ${GREEN}$LAYER1_PASS 通过${NC} / ${RED}$LAYER1_FAIL 失败${NC}"
    echo "  层2（正文内容一致性）: ${GREEN}$LAYER2_PASS 通过${NC} / ${RED}$LAYER2_FAIL 失败${NC}"
    echo "  耗时: ${elapsed}s"
    echo ""

    if [ $FAILED -gt 0 ]; then
        echo "修复方法:"
        echo "  层1 失败 → 在文件的'修改记录'表中添加对应决议编号（如 R-xxx）"
        echo "  层2 must_exist 失败 → 文档正文缺少必需内容——将决议内容写入文档正文对应章节"
        echo "  层2 must_not_exist 失败 → 文档正文仍包含已废弃概念——在正文中清除旧概念"
        echo "  详细模式: 加 --verbose 查看每步执行细节"
    fi
}

# =============================================================================
# 函数: validate_environment
# 用途: 自检——验证脚本运行所需的依赖和文件是否存在。
#       失败 → exit 2（脚本自身错误）
# =============================================================================

validate_environment() {
    log_verbose "自检: 验证运行环境..."

    # 检查必需命令
    for cmd in grep awk sed tail head wc date mktemp; do
        if ! command -v "$cmd" >/dev/null 2>&1; then
            log_error "缺少必需命令: $cmd"
            exit 2
        fi
    done
    log_verbose "  命令依赖: 全部就绪 (grep, awk, sed, tail, head, wc, date, mktemp)"

    # 检查 resolutions.yaml
    if [ ! -f "$RESOLUTIONS" ]; then
        log_error "resolutions.yaml 不存在: $RESOLUTIONS"
        log_error "用法: bash $SCRIPT_NAME [resolutions.yaml路径]"
        exit 2
    fi
    log_verbose "  配置文件: $RESOLUTIONS"

    # 检查 resolutions.yaml 基本格式
    if ! grep -qE '^resolutions:' "$RESOLUTIONS" 2>/dev/null; then
        log_error "resolutions.yaml 格式错误——缺少 'resolutions:' 顶级键"
        exit 2
    fi

    # 检查文档目录
    if [ ! -d "$DOC_DIR" ]; then
        log_error "文档目录不存在: $DOC_DIR（当前目录: $(pwd)）"
        log_error "提示: 请在项目根目录运行此脚本"
        exit 2
    fi
    log_verbose "  文档目录: $DOC_DIR（存在）"

    # 统计决议数量
    local resolution_count
    resolution_count=$(grep -cE '^  R-[0-9]+:' "$RESOLUTIONS" 2>/dev/null || echo 0)
    log_verbose "  决议总数: $resolution_count"
}

# =============================================================================
# main — 脚本入口
# =============================================================================

main() {
    # 参数解析
    RESOLUTIONS="$DEFAULT_RESOLUTIONS"
    while [ $# -gt 0 ]; do
        case "$1" in
            --verbose|-v) VERBOSE=true; shift ;;
            --help|-h)    usage ;;
            --*)          log_error "未知选项: $1。使用 --help 查看用法。"; exit 2 ;;
            *)            RESOLUTIONS="$1"; shift ;;
        esac
    done

    # Banner
    echo ""
    echo -e "${BOLD}=== GoalOS 决议传播完整性检查（两层验证） ===${NC}"
    echo "  配置: $RESOLUTIONS"
    echo "  时间: $(date '+%Y-%m-%d %H:%M:%S')"
    echo ""

    # 自检
    validate_environment

    # 执行两层检查 + 幽灵决议检测
    check_layer1 "$RESOLUTIONS"
    check_layer2 "$RESOLUTIONS"
    check_ghost_resolutions "$RESOLUTIONS"

    # 总结
    print_summary

    # 退出码: 0=全通过, 1=有未传播项
    if [ $FAILED -eq 0 ]; then
        exit 0
    else
        exit 1
    fi
}

main "$@"
