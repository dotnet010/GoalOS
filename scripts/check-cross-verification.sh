#!/bin/bash
# =============================================================================
# GoalOS 四层面交叉验证 — CI 自动化
#
# 四个角色独立扫描，同发现问题取最严格标准。Ken 最终审核。
#   Brad:   实现层面——数据结构字段/协议失败路径/Error Contract
#   Russ:   集成层面——术语追溯/事件注册/数值一致/生命周期
#   Beck:   测试层面——MUST覆盖/否定测试/断言数量
#   Jobs:   产品层面——UI规格/用户旅程/验收标准
#
# 退出码: 0=四层全过, 1=存在任何空白——不得进入开发
# =============================================================================
set -euo pipefail

R='\033[0;31m'; G='\033[0;32m'; Y='\033[1;33m'; N='\033[0m'
FAIL=0
D="开发文档"
A="$D/05软件架构文档.md"; P="$D/01-prd产品需求文档.md"
E="$D/07事件注册表.md"; G="$D/GLOSSARY.md"
HF="$D/03高保真交互原型.md"; DP="$D/开发计划/v0.2.0/v0.2.0-6-开发计划.md"
STUB="$D/stub追踪清单.md"

echo "===== GoalOS 四层面交叉验证（CI 自动化）====="
echo ""

# 提取正文起始行（跳过 frontmatter + 修改记录）
extract_body_start() {
    local fp="$1"
    local start
    start=$(awk '/^---$/{n++; if(n>=2){print NR; exit}}' "$fp" 2>/dev/null)
    if [ -z "$start" ]; then
        start=10
    fi
    echo $((start + 1))
}

A_BODY_START=$(extract_body_start "$A")
G_BODY_START=$(extract_body_start "$G")

# ═══════════════════════════════════════════════════════════════
# BRAD: 实现层面
# ═══════════════════════════════════════════════════════════════
echo "── Brad: 实现层面 —— 数据结构/协议/错误契约 ──"
B_FAIL=0

# 数据结构字段完整性（从正文起始行搜索）
for s in "RecoveryStrategy" "VerificationPrecedence" "FlowSelectionPolicy" "ArbitrationPolicy"; do
  fc=$(tail -n +"$A_BODY_START" "$A" 2>/dev/null | grep -A30 "$s" | grep -cE '[a-z_]+:.*(string|int|bool|float|\[)' || echo 0)
  if [ "${fc:-0}" -ge 2 ]; then echo -e "  ${G}✅${N} $s: $fc 字段"; else echo -e "  ${R}❌${N} $s: 字段不足(需≥2,实${fc:-0})——开发者无法实现"; B_FAIL=$((B_FAIL+1)); fi
done

# 协议失败路径（从正文起始行搜索，检查所有匹配——非仅第一个）
check_failpath() {
  local p="$1"
  local matches
  matches=$(tail -n +"$A_BODY_START" "$A" 2>/dev/null | grep -n "$p" || true)
  if [ -z "$matches" ]; then
    echo -e "  ${R}❌${N} $p: 协议未定义"
    return 1
  fi
  # 遍历所有匹配行，任一处有失败关键词即通过
  local found=0
  while IFS= read -r match_line; do
    local lno
    lno=$(echo "$match_line" | cut -d: -f1)
    local fc
    fc=$(tail -n +"$A_BODY_START" "$A" | tail -n +"$lno" | head -30 | grep -cE "失败|回滚|补偿|ESCALATE|不发布" || echo 0)
    if [ "${fc:-0}" -gt 0 ]; then
      echo -e "  ${G}✅${N} $p: 失败路径${fc}处（正文行$lno）"
      found=1
      break
    fi
  done <<< "$matches"
  if [ "$found" -eq 1 ]; then
    return 0
  else
    local first_lno
    first_lno=$(echo "$matches" | head -1 | cut -d: -f1)
    echo -e "  ${R}❌${N} $p(正文行$first_lno): 无失败路径——所有匹配处均无失败关键词"
    return 1
  fi
}
check_failpath "Wait.*Resume" || B_FAIL=$((B_FAIL+1))
check_failpath "Cancellation" || B_FAIL=$((B_FAIL+1))

# Error Contract（从正文起始行搜索）
for i in "Agent.Align" "Agent.Analyze" "Agent.Plan" "GoalRunner.Execute" "PluginRunner.Execute"; do
  ec=$(tail -n +"$A_BODY_START" "$A" 2>/dev/null | grep -A20 "$i" | grep -cE "Timeout|Retryable|Fatal|Recoverable|ErrorCode" || echo 0)
  if [ "${ec:-0}" -ge 1 ]; then echo -e "  ${G}✅${N} $i: Error分类${ec}处"; else echo -e "  ${R}❌${N} $i: 无Error Contract"; B_FAIL=$((B_FAIL+1)); fi
done

echo "  Brad发现: $B_FAIL 项"
FAIL=$((FAIL + B_FAIL))
echo ""

# ═══════════════════════════════════════════════════════════════
# RUSS: 集成层面
# ═══════════════════════════════════════════════════════════════
echo "── Russ: 集成层面 —— 术语追溯/事件/数值/生命周期 ──"
R_FAIL=0

# 事件注册
for evt in "ProgressUpdate"; do
  if grep -q "$evt" "$DP" 2>/dev/null && ! grep -q "$evt" "$E" 2>/dev/null; then echo -e "  ${R}❌${N} $evt: 开发计划引用,07未注册"; R_FAIL=$((R_FAIL+1)); fi
done
[ $R_FAIL -eq 0 ] && echo -e "  ${G}✅${N} 事件注册完整"

# 数值一致性
af5=$(grep -o '最多.*[0-9].*次\|AUTO_FIX.*[0-9]' "$A" 2>/dev/null | grep -o '[0-9]' | head -1 || echo "?")
afG=$(grep -o 'AUTO_FIX.*最多.*[0-9]' "$G" 2>/dev/null | grep -o '[0-9]' | head -1 || echo "?")
[ "$af5" = "$afG" ] && echo -e "  ${G}✅${N} AUTO_FIX一致($af5)" || { echo -e "  ${R}❌${N} AUTO_FIX漂移:05=$af5,GLOSSARY=$afG"; R_FAIL=$((R_FAIL+1)); }

# BudgetTracker命名
bt=$(grep -c "CircuitBreakerConfig\|circuit_breaker" "$A" 2>/dev/null || echo 0)
[ "$bt" -gt 0 ] && echo -e "  ${Y}⚠️${N} BudgetTracker/CircuitBreakerConfig命名不一致"

echo "  Russ发现: $R_FAIL 项"
FAIL=$((FAIL + R_FAIL))
echo ""

# ═══════════════════════════════════════════════════════════════
# BECK: 测试层面
# ═══════════════════════════════════════════════════════════════
echo "── Beck: 测试层面 —— 覆盖/否定测试/断言 ──"
BK_FAIL=0

# Smoke Test 存在性
grep -q "Smoke Test\|TestSmoke_MinimalUserJourney" "$DP" 2>/dev/null && echo -e "  ${G}✅${N} Smoke Test已定义" || { echo -e "  ${R}❌${N} Smoke Test缺失(R-615)"; BK_FAIL=$((BK_FAIL+1)); }

# MUST_NOT 否定测试覆盖——关键项检查
for mn in "同步阻塞等待子进程" "注入.*secret_key.*子进程" "跳过引擎" "核心层禁止I/O"; do
  found=$(grep -c "$mn" "$DP" 2>/dev/null || echo 0)
  [ "${found:-0}" -eq 0 ] && echo -e "  ${Y}⚠️${N} MUST_NOT '$mn': 否定测试需人工确认"
done

# TC→stub追溯
tc_undef=0
for tc in $(grep -ohE 'TC-[A-Z]+-[0-9]+' "$DP" 2>/dev/null | sort -u); do
  grep -q "$tc" "$STUB" 2>/dev/null || tc_undef=$((tc_undef + 1))
done
[ "$tc_undef" -gt 0 ] && echo -e "  ${R}❌${N} $tc_undef 个TC编号在stub追踪中无定义(K顾问P0-1)" && BK_FAIL=$((BK_FAIL+1))
[ "$tc_undef" -eq 0 ] && echo -e "  ${G}✅${N} 所有TC编号可追溯到stub追踪"

echo "  Beck发现: $BK_FAIL 项"
FAIL=$((FAIL + BK_FAIL))
echo ""

# ═══════════════════════════════════════════════════════════════
# JOBS: 产品层面
# ═══════════════════════════════════════════════════════════════
echo "── Jobs: 产品层面 —— UI规格/用户旅程/验收 ──"
J_FAIL=0

# CompletionContract UI
if grep -q "CompletionContract\|成功标准.*验收条件\|必须产出物.*约束条件" "$HF" 2>/dev/null; then echo -e "  ${G}✅${N} CompletionContract UI"; else echo -e "  ${R}❌${N} CompletionContract UI缺失——R-516未兑现"; J_FAIL=$((J_FAIL+1)); fi

# HumanIntervention 选项一致性
hi_g=$(grep -c "继续等待\|简化方案\|更换模型\|取消目标" "$G" 2>/dev/null || echo 0)
hi_ui=$(grep -c "继续等待\|简化方案\|更换模型\|取消目标" "$HF" 2>/dev/null || echo 0)
[ "$hi_g" -gt 0 ] && [ "$hi_ui" -eq 0 ] && { echo -e "  ${R}❌${N} HumanIntervention: GLOSSARY有选项,UI无"; J_FAIL=$((J_FAIL+1)); }
[ "$hi_g" -gt 0 ] && [ "$hi_ui" -gt 0 ] && echo -e "  ${G}✅${N} HumanIntervention选项一致"

# failHints 枚举完整性
fh_d=$(grep -o '[0-9]*.种.*failHints\|failHints.*[0-9]*.种' "$P" 2>/dev/null | grep -o '[0-9]' | head -1 || echo "0")
fh_a=$(grep -cE '^\|.*\|.*\|.*\|$' "$P" 2>/dev/null || echo 0)
if [ "${fh_d:-0}" -gt "${fh_a:-0}" ] 2>/dev/null; then echo -e "  ${R}❌${N} failHints: 宣称${fh_d}种,枚举${fh_a}种"; J_FAIL=$((J_FAIL+1)); else echo -e "  ${G}✅${N} failHints枚举完整"; fi

echo "  Jobs发现: $J_FAIL 项"
FAIL=$((FAIL + J_FAIL))
echo ""

# ═══════════════════════════════════════════════════════════════
# KEN: 交叉验证 + 最终裁决
# ═══════════════════════════════════════════════════════════════
echo "── Ken: 交叉验证 —— 重叠发现取最严格 ──"
# 检查Brad和Russ是否有重叠发现(同一概念两人都发现)
overlap=0
for c in "RecoveryStrategy" "VerificationPrecedence" "FlowSelectionPolicy"; do
  b_found=$(tail -n +"$A_BODY_START" "$A" 2>/dev/null | grep -c "$c" || echo 0)
  r_found=$(tail -n +"$G_BODY_START" "$G" 2>/dev/null | grep -c "$c" || echo 0)
  [ "$b_found" -gt 0 ] && [ "$r_found" -gt 0 ] && { echo -e "  ${Y}⚠️${N} $c: Brad(实现层)+Russ(GLOSSARY可追溯)→重叠发现。Brad层已验证字段完整性。"; overlap=$((overlap+1)); }
done
[ "$overlap" -eq 0 ] && echo -e "  ${G}✅${N} 无重叠发现——四人发现独立互补"

echo ""
echo "══════════════════════════════"
echo "  Brad(实现): $B_FAIL  |  Russ(集成): $R_FAIL"
echo "  Beck(测试): $BK_FAIL  |  Jobs(产品): $J_FAIL"
echo "  交叉重叠: $overlap    |  合计失败: $FAIL"
echo "══════════════════════════════"
echo ""

if [ $FAIL -eq 0 ]; then
    echo -e "${G}✅ 四层面交叉验证全部通过。可进入开发。${N}"
    exit 0
else
    echo -e "${R}❌ $FAIL 项空白。四层面任一项不通过→不得进入开发。${N}"
    echo "修复按层面分类: Brad→数据结构+协议 | Russ→追溯+事件 | Beck→测试覆盖 | Jobs→UI+验收"
    exit 1
fi
