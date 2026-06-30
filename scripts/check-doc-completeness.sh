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
#
# 退出码: 0=全部通过, 1=存在规格空白
# =============================================================================
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
FAILED=0
D="开发文档"
A="$D/05软件架构文档.md"; P="$D/01-prd产品需求文档.md"
E="$D/07事件注册表.md"; G="$D/GLOSSARY.md"
HF="$D/03高保真交互原型.md"; DP="$D/开发计划/v0.2.0/v0.2.0-6-开发计划.md"

echo "=== 开发文档完整性检查 — 七类细节缺失 ==="
echo ""

# ═══ A: Referenced-but-Undefined ═══
echo "── A. 被引用但无字段级定义 ──"
A_FAIL=0
check_defined() { local name="$1" file="$2" min_fields="${3:-2}"; local fc; fc=$(grep -A30 "$name" "$file" 2>/dev/null | grep -cE '^\s+[a-z_]+:.*#' || echo 0); if [ "$fc" -lt "$min_fields" ]; then echo -e "  ${RED}❌${NC} $name: 仅 $fc 字段级定义 (需≥$min_fields)"; return 1; else echo -e "  ${GREEN}✅${NC} $name: $fc 字段"; return 0; fi; }
check_defined "RecoveryStrategy" "$A" 2 || A_FAIL=$((A_FAIL+1))
check_defined "DisambiguationPolicy" "$A" 2 || A_FAIL=$((A_FAIL+1))
check_defined "RecoverySelector" "$A" 2 || A_FAIL=$((A_FAIL+1))
check_defined "VerificationPrecedence" "$A" 2 || A_FAIL=$((A_FAIL+1))
check_defined "FlowSelectionPolicy" "$A" 2 || A_FAIL=$((A_FAIL+1))
# GLOSSARY→05 trace
for term in "Consumer Cursor" "GoalAnchor"; do
  if grep -q "$term" "$G" 2>/dev/null && ! grep -q "$term" "$A" 2>/dev/null; then
    echo -e "  ${RED}❌${NC} $term: GLOSSARY有定义,05-架构无引用"; A_FAIL=$((A_FAIL+1))
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
# failHints
fh_decl=$(grep -oP 'failHints.*?\d+' "$P" 2>/dev/null | grep -oP '\d+' | head -1 || echo "0")
fh_actual=$(grep -cE '\|.*\|.*\|.*\|' "$P" 2>/dev/null || echo 0)
if [ "${fh_decl:-0}" -gt "${fh_actual:-0}" ] 2>/dev/null; then
  echo -e "  ${RED}❌${NC} failHints: 宣称${fh_decl}种,实际枚举${fh_actual}种"; B_FAIL=$((B_FAIL+1))
else echo -e "  ${GREEN}✅${NC} failHints: 枚举完整"; fi
# HumanIntervention options consistency
hi_g=$(grep -c "继续等待\|简化方案\|更换模型\|取消目标" "$G" 2>/dev/null || echo 0)
hi_ui=$(grep -c "继续等待\|简化方案\|更换模型\|取消目标" "$HF" 2>/dev/null || echo 0)
if [ "$hi_g" -gt 0 ] && [ "$hi_ui" -eq 0 ]; then
  echo -e "  ${RED}❌${NC} HumanIntervention: GLOSSARY有4选项,03-高保真无对应"; B_FAIL=$((B_FAIL+1))
else echo -e "  ${GREEN}✅${NC} HumanIntervention: 选项一致"; fi
FAILED=$((FAILED + B_FAIL))
echo ""

# ═══ C: Cross-Document Drift ═══
echo "── C. 跨文档数值漂移 ──"
C_FAIL=0
af_a=$(grep -oP 'AUTO_FIX.*?\K[0-9]+' "$A" 2>/dev/null | head -1 || echo "?")
af_g=$(grep -oP 'AUTO_FIX.*?\K[0-9]+' "$G" 2>/dev/null | head -1 || echo "?")
if [ "$af_a" = "$af_g" ]; then echo -e "  ${GREEN}✅${NC} AUTO_FIX: 一致($af_a)"; else echo -e "  ${RED}❌${NC} AUTO_FIX: 05=$af_a vs GLOSSARY=$af_g"; C_FAIL=$((C_FAIL+1)); fi
# BudgetTracker naming
bt_names=$(grep -c "CircuitBreakerConfig\|circuit_breaker.*yaml" "$A" 2>/dev/null || echo 0)
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
  fc=$(tail -n +"$lno" "$A" | head -30 | grep -cE "失败|回滚|补偿|ESCALATE|不发布" || echo 0)
  if [ "$fc" -eq 0 ]; then echo -e "  ${RED}❌${NC} '$proto'(行$lno): 无失败路径"; D_FAIL=$((D_FAIL+1)); fi
done
[ $D_FAIL -eq 0 ] && echo -e "  ${GREEN}✅${NC} 已检查协议均有失败路径"
FAILED=$((FAILED + D_FAIL))
echo ""

# ═══ E: Missing UI Interaction ═══
echo "── E. UI交互规格缺失 ──"
E_FAIL=0
if grep -q "CompletionContract\|成功标准.*验收条件" "$HF" 2>/dev/null; then echo -e "  ${GREEN}✅${NC} CompletionContract: 交互规格存在"; else echo -e "  ${RED}❌${NC} CompletionContract: 交互规格缺失(R-617)"; E_FAIL=$((E_FAIL+1)); fi
FAILED=$((FAILED + E_FAIL))
echo ""

# ═══ F: Missing Lifecycle ═══
echo "── F. 组件生命周期缺失 ──"
F_FAIL=0
for mod in "BudgetTracker" "Snapshot"; do
  lc=$(grep -A50 "$mod" "$A" 2>/dev/null | grep -cE "Init|启动|Shutdown|关闭|Recover|恢复|重启" || echo 0)
  if [ "$lc" -ge 2 ]; then echo -e "  ${GREEN}✅${NC} $mod: 生命周期已描述($lc处)"; else echo -e "  ${RED}❌${NC} $mod: 生命周期不足($lc处)"; F_FAIL=$((F_FAIL+1)); fi
done
FAILED=$((FAILED + F_FAIL))
echo ""

# ═══ G: Missing Error Contract ═══
echo "── G. Error Contract缺失 ──"
G_FAIL=0
for iface in "Agent.Analyze" "GoalRunner.Execute"; do
  ec=$(grep -A20 "$iface" "$A" 2>/dev/null | grep -cE "error|Error|返回.*err|Timeout|Retry|Fatal" || echo 0)
  if [ "$ec" -ge 2 ]; then echo -e "  ${GREEN}✅${NC} $iface: error描述${ec}处"; else echo -e "  ${RED}❌${NC} $iface: error描述仅${ec}处——缺少Error Contract"; G_FAIL=$((G_FAIL+1)); fi
done
FAILED=$((FAILED + G_FAIL))
echo ""

# ═══ 总结 ═══
echo "──────────────────────────────"
echo "  A(未定义): $A_FAIL  | B(未穷举): $B_FAIL  | C(漂移): $C_FAIL"
echo "  D(无失败): $D_FAIL  | E(无UI):   $E_FAIL  | F(无生命周期): $F_FAIL"
echo "  G(无Error): $G_FAIL"
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

echo "  七标准(A-G): $((FAILED - PLACEHOLDER_FAIL))  | Placeholder残留: $PLACEHOLDER_FAIL"
echo "  失败合计: $FAILED"
echo ""

[ $FAILED -eq 0 ] && echo -e "${GREEN}✅ 文档完整性检查全部通过——可送顾问审计${NC}" && exit 0
echo -e "${RED}❌ $FAILED 项规格空白/未完成标记。R-628+R-634: 缺失→不得进入开发→不得送顾问审计。${NC}"
echo "修复: 每个❌对应一个具体缺失。按A-G+R-634分类查找对应文档。"
exit 1
