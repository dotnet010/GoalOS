#!/bin/bash
# =============================================================================
# GoalOS 开发文档完整性检查（R-629）
#
# 七类细节缺失——不是凭空假设，是从六轮顾问审计+全量文档扫描中抽象:
#   A. Referenced-but-Undefined — 被引用但字段级定义缺失
#   B. Declared-but-Unenumerated — 声明"N种"但未列出全部N个值
#   C. Cross-Document Drift — 同一概念在不同文档中数值/定义不一致
#   D. Happy-Path-Only — 协议定义了成功路径但缺少失败回滚
#   E. Missing UI Interaction — 交互组件有名称无状态机
#   F. Missing Lifecycle — 组件被大量引用但无Init/Run/Shutdown/Recover
#   G. Missing Error Contract — 接口返回error但无错误类型分类
#   H. 任务引用可解析 — '任务 X.Y' 引用必须可解析于 v0.3.0 计划任务表
#      （S'-27 R-1270，会议 #200 验收标准机测化——重排负责人=PM；
#       '原任务 X.Y' 重命名别名可解析；会议纪要/历史计划排除）
#
# 退出码: 0=全部通过, 1=存在规格空白
# 最后更新: 2026-08-13（会议 #200 S'-27 R-1270——新增 H 类任务引用可解析检查；
#            会议 #107 R-740——A/C 段已废除概念（RecoveryStrategy/RecoverySelector/
#            AUTO_FIX）转废除语境冻结检查；grep -c||echo 0 双零伪值修复改 || true；
#            S'-24⑥ R-1267 裁决接线——补 R-1052 三布局自适应（repo-only 显式降级））
# =============================================================================
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
FAILED=0
# ── R-1052 三布局自适应（S'-24⑥ 裁决接线后必需）──
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
if [ -d "开发文档" ]; then
  WS="$PWD"
elif [ -d "$SCRIPT_DIR/../../开发文档" ]; then
  WS="$(cd "$SCRIPT_DIR/../.." && pwd)"
else
  echo -e "${YELLOW}⚠️${NC} 未定位工作区布局（无 开发文档/）——repo-only 布局显式降级: ⏭️ SKIP 不追溯"
  exit 0
fi
D="$WS/开发文档"
A="$D/05软件架构文档.md"; P="$D/01-prd产品需求文档.md"
E="$D/07事件注册表.md"; G="$D/00统一术语表.md"
HF="$D/03高保真交互原型.md"; DP="$D/开发计划/v0.2.0/v0.2.0-6-开发计划.md"

echo "=== 开发文档完整性检查 — 七类细节缺失 ==="
echo ""

# ═══ A: Referenced-but-Undefined ═══
echo "── A. 被引用但无字段级定义 ──"
A_FAIL=0
# 定义细节计数: YAML 字段行（name: value #comment）或散文参数标记
# （最大 N/每轮/停止条件/默认值——05 的 DisambiguationPolicy 为单行散文定义格式）
check_defined() { local name="$1" file="$2" min_fields="${3:-2}"; local fc; fc=$(grep -A30 "$name" "$file" 2>/dev/null | grep -oE '[a-z_]+:.*#|最大 [0-9]+|每轮|停止条件|默认值' 2>/dev/null | wc -l | tr -d ' ' || true); if [ "$fc" -lt "$min_fields" ]; then echo -e "  ${RED}❌${NC} $name: 仅 $fc 处定义细节 (需≥$min_fields)"; return 1; else echo -e "  ${GREEN}✅${NC} $name: $fc 处定义细节"; return 0; fi; }
check_defined "DisambiguationPolicy" "$A" 2 || A_FAIL=$((A_FAIL+1))
check_defined "VerificationPrecedence" "$A" 2 || A_FAIL=$((A_FAIL+1))
check_defined "FlowSelectionPolicy" "$A" 2 || A_FAIL=$((A_FAIL+1))
# RecoveryStrategy/RecoverySelector 已废除（R-740 会议 #107）——不查字段定义，
# 改为废除语境冻结检查（00统一术语表 保留条目标注已废除；两文档正文提及必须处于废除语境）
for term in "RecoveryStrategy" "RecoverySelector"; do
  for f in "$A" "$G"; do
    astart=$(awk '/^---$/{n++; if(n>=2){print NR; exit}}' "$f" 2>/dev/null)
    [ -z "$astart" ] && astart=10
    bad=$(tail -n +"$((astart + 1))" "$f" 2>/dev/null | grep -n "$term" | grep -v "废除\|废弃\|不再有" || true)
    if [ -n "$bad" ]; then
      echo -e "  ${RED}❌${NC} $term($(basename "$f")): 提及未处废除语境（R-740）"; A_FAIL=$((A_FAIL+1))
    fi
  done
done
# 00统一术语表→05 trace
for term in "Consumer Cursor" "GoalAnchor"; do
  if grep -q "$term" "$G" 2>/dev/null && ! grep -q "$term" "$A" 2>/dev/null; then
    echo -e "  ${RED}❌${NC} $term: 00统一术语表有定义,05-架构无引用"; A_FAIL=$((A_FAIL+1))
  fi
done
# Event注册
for evt in "ProgressUpdate"; do
  if grep -q "$evt" "$DP" 2>/dev/null && ! grep -q "$evt" "$E" 2>/dev/null; then
    echo -e "  ${RED}❌${NC} $evt: 开发计划引用,07-事件未注册"; A_FAIL=$((A_FAIL+1))
  fi
done
FAILED=$((FAILED + A_FAIL))
echo ""

# ═══ B: Declared-but-Unenumerated ═══
echo "── B. 声明数量但未穷举 ──"
B_FAIL=0
# failHints（macOS BSD grep 无 -P，用 -Eo 两段提取）
fh_decl=$(grep -Eo 'failHints[^0-9]*[0-9]+' "$P" 2>/dev/null | grep -Eo '[0-9]+' | head -1 || echo "0")
fh_actual=$(grep -cE '\|.*\|.*\|.*\|' "$P" 2>/dev/null || true)
if [ "${fh_decl:-0}" -gt "${fh_actual:-0}" ] 2>/dev/null; then
  echo -e "  ${RED}❌${NC} failHints: 宣称${fh_decl}种,实际枚举${fh_actual}种"; B_FAIL=$((B_FAIL+1))
else echo -e "  ${GREEN}✅${NC} failHints: 枚举完整"; fi
# HumanIntervention options consistency（03 已归档——同 check-doc-version.sh 归档 SKIP 原则）
hi_g=$(grep -c "继续等待\|简化方案\|更换模型\|取消目标" "$G" 2>/dev/null || true)
if [ ! -f "$HF" ]; then
  echo -e "  ${YELLOW}⚠️${NC} HumanIntervention: 03高保真交互原型已归档（废弃文档回收站）——⏭️ SKIP 不追溯"
elif [ "$hi_g" -gt 0 ] && ! grep -q "继续等待\|简化方案\|更换模型\|取消目标" "$HF" 2>/dev/null; then
  echo -e "  ${RED}❌${NC} HumanIntervention: 00统一术语表有4选项,03-高保真无对应"; B_FAIL=$((B_FAIL+1))
else echo -e "  ${GREEN}✅${NC} HumanIntervention: 选项一致"; fi
FAILED=$((FAILED + B_FAIL))
echo ""

# ═══ C: Cross-Document Drift ═══
echo "── C. 跨文档数值漂移 ──"
C_FAIL=0
# AUTO_FIX 语义冻结（R-740 会议 #107——RETRY/REPLAN/AUTO_FIX 废除，新 Session 重做替代）。
# 原"重试次数跨文档一致"数值契约随废除消亡（残留数值均为修改记录历史）；
# 改为检查两文档正文中 AUTO_FIX 提及必须处于废除语境——防旧恢复语义残留复活
af_bad=""
for f in "$A" "$G"; do
  astart=$(awk '/^---$/{n++; if(n>=2){print NR; exit}}' "$f" 2>/dev/null)
  [ -z "$astart" ] && astart=10
  hits=$(tail -n +"$((astart + 1))" "$f" 2>/dev/null | grep -n "AUTO_FIX" | grep -v "废除\|废弃\|不再有" || true)
  [ -n "$hits" ] && af_bad="$af_bad $(basename "$f"):$(echo "$hits" | head -1 | cut -d: -f1)"
done
if [ -z "$af_bad" ]; then echo -e "  ${GREEN}✅${NC} AUTO_FIX: 两文档一致标记废除（R-740）"; else echo -e "  ${RED}❌${NC} AUTO_FIX 提及未处废除语境（R-740）:$af_bad"; C_FAIL=$((C_FAIL+1)); fi
# BudgetTracker naming
bt_names=$(grep -c "CircuitBreakerConfig\|circuit_breaker.*yaml" "$A" 2>/dev/null || true)
if [ "$bt_names" -gt 0 ]; then echo -e "  ${YELLOW}⚠️${NC} BudgetTracker/CircuitBreakerConfig命名不一致(R-550)"; fi
FAILED=$((FAILED + C_FAIL))
echo ""

# ═══ D: Happy-Path-Only ═══
echo "── D. 协议失败路径缺失 ──"
D_FAIL=0
for proto in "Token.*续期" "MissionNode.*Action.*转换" "Wait.*Resume"; do
  sec=$(grep -n "$proto" "$A" 2>/dev/null | head -1 || true)
  [ -z "$sec" ] && continue
  lno=$(echo "$sec" | cut -d: -f1)
  fc=$(tail -n +"$lno" "$A" | head -30 | grep -cE "失败|回滚|补偿|ESCALATE|不发布" || true)
  if [ "$fc" -eq 0 ]; then echo -e "  ${RED}❌${NC} '$proto'(行$lno): 无失败路径"; D_FAIL=$((D_FAIL+1)); fi
done
[ $D_FAIL -eq 0 ] && echo -e "  ${GREEN}✅${NC} 已检查协议均有失败路径"
FAILED=$((FAILED + D_FAIL))
echo ""

# ═══ E: Missing UI Interaction ═══
echo "── E. UI交互规格缺失 ──"
E_FAIL=0
if [ ! -f "$HF" ]; then
  echo -e "  ${YELLOW}⚠️${NC} CompletionContract: 03高保真交互原型已归档（废弃文档回收站）——⏭️ SKIP 不追溯"
elif grep -q "CompletionContract\|成功标准.*验收条件" "$HF" 2>/dev/null; then echo -e "  ${GREEN}✅${NC} CompletionContract: 交互规格存在"; else echo -e "  ${RED}❌${NC} CompletionContract: 交互规格缺失(R-617)"; E_FAIL=$((E_FAIL+1)); fi
FAILED=$((FAILED + E_FAIL))
echo ""

# ═══ F: Missing Lifecycle ═══
echo "── F. 组件生命周期缺失 ──"
F_FAIL=0
for mod in "BudgetTracker" "Snapshot"; do
  lc=$(grep -A50 "$mod" "$A" 2>/dev/null | grep -cE "Init|启动|Shutdown|关闭|Recover|恢复|重启" || true)
  if [ "$lc" -ge 2 ]; then echo -e "  ${GREEN}✅${NC} $mod: 生命周期已描述($lc处)"; else echo -e "  ${RED}❌${NC} $mod: 生命周期不足($lc处)"; F_FAIL=$((F_FAIL+1)); fi
done
FAILED=$((FAILED + F_FAIL))
echo ""

# ═══ G: Missing Error Contract ═══
echo "── G. Error Contract缺失 ──"
G_FAIL=0
for iface in "Agent.Analyze" "GoalRunner.Execute"; do
  ec=$(grep -A20 "$iface" "$A" 2>/dev/null | grep -cE "error|Error|返回.*err|Timeout|Retry|Fatal" || true)
  if [ "$ec" -ge 2 ]; then echo -e "  ${GREEN}✅${NC} $iface: error描述${ec}处"; else echo -e "  ${RED}❌${NC} $iface: error描述仅${ec}处——缺少Error Contract"; G_FAIL=$((G_FAIL+1)); fi
done
FAILED=$((FAILED + G_FAIL))
echo ""

# ═══ H: 任务引用可解析（S'-27 R-1270——验收标准机测化）═══
# 校验 resolutions.yaml 全部 desc 与文档正文中 '任务 X.Y' 引用在 v0.3.0 计划任务表可解析。
# 可解析集合 = 任务表行编号（^| X.Y |）+ '原任务 X.Y' 重命名别名（如 1.14→3.18, R-1068）。
# 扫描范围: resolutions.yaml 全文 + 开发文档顶层/待审议规范/v0.3.0 计划目录正文
# （跳过 frontmatter+修改记录——同 check-resolution-propagation.sh 正文提取契约；
#   排除 会议纪要.md 历史记录 / *.bak.md / v0.2.0 历史计划）。
echo "── H. 任务引用可解析（'任务 X.Y' 引用 ∈ 计划任务表）──"
H_FAIL=0
PLAN_FILE=$(ls "$D"/开发计划/v0.3.0/v0.3.0-*-开发计划.md 2>/dev/null | head -1 || true)
if [ -z "$PLAN_FILE" ] || [ ! -f "$PLAN_FILE" ]; then
    echo -e "  ${YELLOW}⚠️${NC} 未找到 v0.3.0 计划文件——任务引用可解析检查跳过"
else
    resolvable_tasks=$(mktemp); task_refs=$(mktemp)
    # 可解析集合: 任务表编号 + '原任务 X.Y' 重命名别名
    grep -E '^\| *[0-9]+\.[0-9]+ *\|' "$PLAN_FILE" 2>/dev/null | grep -oE '[0-9]+\.[0-9]+' | sort -u > "$resolvable_tasks"
    grep -oE '原任务 *[0-9]+\.[0-9]+' "$PLAN_FILE" 2>/dev/null | grep -oE '[0-9]+\.[0-9]+' | sort -u >> "$resolvable_tasks"
    # 引用来源 1: resolutions.yaml 全部 desc（工作区布局: GoalOS/resolutions.yaml）
    grep -oE '任务 *[0-9]+\.[0-9]+' "$WS/GoalOS/resolutions.yaml" 2>/dev/null | grep -oE '[0-9]+\.[0-9]+' | sort -u > "$task_refs"
    # 引用来源 2: 文档正文
    for fp in "$D"/*.md "$D"/待审议规范/*.md "$D"/开发计划/v0.3.0/*.md; do
        [ -f "$fp" ] || continue
        case "$fp" in
            *会议纪要.md|*.bak.md) continue ;;
        esac
        start=$(awk '/^---$/{n++; if(n>=2){print NR; exit}}' "$fp" 2>/dev/null)
        if [ -z "$start" ]; then start=10; fi
        tail -n +"$((start + 1))" "$fp" 2>/dev/null \
            | grep -oE '任务 *[0-9]+\.[0-9]+' 2>/dev/null \
            | grep -oE '[0-9]+\.[0-9]+' 2>/dev/null || true
    done | sort -u >> "$task_refs"
    ref_count=$(wc -l < "$task_refs" | tr -d ' ')
    while IFS= read -r t; do
        [ -z "$t" ] && continue
        if ! grep -qx "$t" "$resolvable_tasks"; then
            echo -e "  ${RED}❌${NC} 任务 $t: 被引用但不在计划任务表（不可解析）"
            H_FAIL=$((H_FAIL+1))
        fi
    done < "$task_refs"
    rm -f "$resolvable_tasks" "$task_refs"
    [ "$H_FAIL" -eq 0 ] && echo -e "  ${GREEN}✅${NC} 全部任务引用可解析（$ref_count 个不同任务号）"
fi
FAILED=$((FAILED + H_FAIL))
echo ""

# ═══ 总结 ═══
echo "──────────────────────────────"
echo "  A(未定义): $A_FAIL  | B(未穷举): $B_FAIL  | C(漂移): $C_FAIL"
echo "  D(无失败): $D_FAIL  | E(无UI):   $E_FAIL  | F(无生命周期): $F_FAIL"
echo "  G(无Error): $G_FAIL | H(任务引用): $H_FAIL"
echo ""

# ═══ R-634: Placeholder/TBD 残留检测 ═══
echo "── R-634: Placeholder/TBD/未完成标记检测 ──"
PLACEHOLDER_FAIL=0
for f in "$A" "$P" "$G" "$HF" "$D/架构会议规范.md" "$DP"; do
    [ ! -f "$f" ] && continue
    found=$(grep -n "placeholder.*标注\|待补充\|TODO\|TBD\|❌未定义" "$f" 2>/dev/null | grep -v "修改记录\|CI 决议\|参考文献\|已兑现\|已完成\|placeholder 标注" | head -5 || true)
    if [ -n "$found" ]; then
        echo -e "  ${RED}❌${NC} $(basename "$f"): 存在未完成标记"
        echo "$found" | while read -r line; do echo "    $line"; done
        PLACEHOLDER_FAIL=$((PLACEHOLDER_FAIL + 1))
    fi
done
[ $PLACEHOLDER_FAIL -eq 0 ] && echo -e "  ${GREEN}✅${NC} 所有文档无未完成标记"
FAILED=$((FAILED + PLACEHOLDER_FAIL))
echo ""

# ═══ I: 会议纪要决议追溯完整性（v3.11——PM 裁定：以后再也没有草稿）═══
# 规则：会议记录直接写入会议纪要.md，不设草稿区。待审议规范/ 目录中不得存在
#   任何会议草稿文件（工作笔记/补闭裁决/讨论清单/审计清单/顾问汇总/阶段4落地清单）。
echo "── I. 会议纪要决议追溯完整性（禁草稿）──"
I_FAIL=0
DRAFTS_DIR="$D/待审议规范"
if [ -d "$DRAFTS_DIR" ]; then
    draft_files=$(ls "$DRAFTS_DIR" 2>/dev/null | grep -E "会议|工作笔记|补闭|讨论清单|审计清单|顾问|阶段4" || true)
    if [ -n "$draft_files" ]; then
        echo -e "  ${RED}❌${NC} 待审议规范存在会议草稿文件（PM 裁定 2026-08-14：以后再也没有草稿——会议记录直接写入会议纪要.md）:"
        echo "$draft_files" | head -10 | sed 's/^/    /'
        I_FAIL=1
    else
        echo -e "  ${GREEN}✅${NC} 无会议草稿文件（禁草稿裁定生效）"
    fi
else
    echo -e "  ${GREEN}✅${NC} 待审议规范目录不存在（禁草稿裁定生效）"
fi
FAILED=$((FAILED + I_FAIL))
echo ""

# ═══ J: 概念锚点表（R-1411——决议修改集合时必须回填权威定义；C 顾问流程建议采纳）═══
# 每行: 概念 | 文件 | 权威定义短语（must_exist——短语本身编码"取值/字段集合完整"）
# 当决议修改某枚举/结构体取值集合时，权威短语必须随决议号回填更新，否则本段红。
echo "── J. 概念锚点表（权威定义完整性）──"
J_FAIL=0
J_ANCHORS=(
  "GoalState 9 态|05软件架构文档.md|Stopped/Failed/Cancelled → Completed（9 态"
  "ActionState 终态 4 值|05软件架构文档.md|终态=Completed/Failed/Timeout/Cancelled"
  "PipelineState 五元组|05软件架构文档.md|ResumePrimitive, ResumePoint, PendingWaits, TimeoutAt, TimeoutKind"
  "ApprovalState 三值|05软件架构文档.md|不设独立超时终态"
  "端点家族单一权威|05软件架构文档.md|端点家族单一权威"
  "信封 13 字段|05软件架构文档.md|prev_hash"
  "reject_reason 十值|07事件注册表.md|governance_denied"
)
for row in "${J_ANCHORS[@]}"; do
    name="${row%%|*}"; rest="${row#*|}"
    f="${rest%%|*}"; phrase="${rest#*|}"
    fp="$D/$f"
    if [ ! -f "$fp" ]; then
        echo -e "  ${RED}❌${NC} $name: 锚定文件不存在 $f"
        J_FAIL=$((J_FAIL+1)); continue
    fi
    if grep -q "$phrase" "$fp" 2>/dev/null; then
        echo -e "  ${GREEN}✅${NC} $name"
    else
        echo -e "  ${RED}❌${NC} $name: 权威定义短语缺失——"$phrase" 不在 $f（决议修改集合必须回填权威定义，R-1411）"
        J_FAIL=$((J_FAIL+1))
    fi
done
FAILED=$((FAILED + J_FAIL))
echo ""

# ═══ K: 会议记录质量闸（v3.11——直接查会议纪要.md 最新会议区；防"结论式记录"复发）═══
# 范围：会议纪要.md 中最后一个"会议 #"标题（含"并入"）到文件末尾的区块。
# 规则 1（决议块完整性）：区块内每个 "### 决议 R-" 块必须含 "第一性原理依据" 与 "妥协审查"。
# 规则 2（妥协红旗词）："过渡/暂缓/先这样" 出现行必须含 "妥协|零容忍|撤销|已纠正" 标注。
echo "── K. 会议记录质量闸（会议纪要最新会议区）──"
K_FAIL=0
MINUTES_F="$D/会议纪要.md"
# 定位"含决议块的最近会议区"：先找全文最后一个 "### 决议 R-" 行号，
# 再找其之前的最近一个会议标题行（## [并入 或 # 会议 #）作为区域起点。
last_res=$(grep -n "^### 决议 R-" "$MINUTES_F" 2>/dev/null | tail -1 | cut -d: -f1 || true)
last_heading=""
if [ -n "$last_res" ]; then
    last_heading=$(head -n "$last_res" "$MINUTES_F" 2>/dev/null | grep -n "^## [并入" 2>/dev/null | tail -1 | cut -d: -f1 || true)
    if [ -z "$last_heading" ]; then
        last_heading=$(head -n "$last_res" "$MINUTES_F" 2>/dev/null | grep -n "^# 会议 #" 2>/dev/null | tail -1 | cut -d: -f1 || true)
    fi
fi
if [ -n "$last_heading" ]; then
    latest=$(tail -n +"$last_heading" "$MINUTES_F")
    rcount=$(echo "$latest" | grep -c "^### 决议 R-" || true)
    fp_count=$(echo "$latest" | grep -c "第一性原理依据" || true)
    comp_count=$(echo "$latest" | grep -c "妥协审查" || true)
    if [ "$rcount" -gt "$fp_count" ] || [ "$rcount" -gt "$comp_count" ]; then
        echo -e "  ${RED}❌${NC} 会议纪要最新会议区: 决议 ${rcount} 块 vs 第一性原理依据 ${fp_count} 处/妥协审查 ${comp_count} 处——结论式记录（v3.10 禁止）"
        K_FAIL=1
    else
        echo -e "  ${GREEN}✅${NC} 会议纪要最新会议区: ${rcount} 决议块字段完整"
    fi
    # 规则 2 范围=决议块区域（讨论区合法引用红旗词概念，如增补讨论的元讨论——不误伤）
    res_regions=$(awk '/^### 决议 R-/{f=1} /^## /{f=0} f' "$MINUTES_F")
    red_flags=$(echo "$res_regions" | grep -nE "过渡方案|暂缓|先这样" | grep -vE "妥协|零容忍|撤销|已纠正" | head -3 || true)
    if [ -n "$red_flags" ]; then
        echo -e "  ${RED}❌${NC} 决议块内妥协红旗词未标注分类: $(echo "$red_flags" | head -2 | tr '\n' ' ')"
        K_FAIL=1
    fi
else
    echo -e "  ${YELLOW}⚠️${NC} 会议纪要未定位到会议区标题——K 段跳过"
fi
FAILED=$((FAILED + K_FAIL))

echo ""

echo "  八标准(A-H): $((FAILED - PLACEHOLDER_FAIL - I_FAIL - J_FAIL - K_FAIL))  | Placeholder残留: $PLACEHOLDER_FAIL  | 会议纪要追溯(I): $I_FAIL  | 概念锚点(J): $J_FAIL  | 会议记录质量(K): $K_FAIL"
echo "  失败合计: $FAILED"
echo ""

[ $FAILED -eq 0 ] && echo -e "${GREEN}✅ 文档完整性检查全部通过——可送顾问审计${NC}" && exit 0
echo -e "${RED}❌ $FAILED 项规格空白/未完成标记。R-628+R-634: 缺失→不得进入开发→不得送顾问审计。${NC}"
echo "修复: 每个❌对应一个具体缺失。按A-G+R-634分类查找对应文档。"
exit 1
