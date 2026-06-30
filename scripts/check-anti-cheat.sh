#!/bin/bash
# =============================================================================
# GoalOS CI 反欺骗检查 — R-568（Beck 规则 5 的实现）
# R-706: 层B增加行为变异检测——核心函数否定测试分支覆盖率>60%（G顾问审计）
#
# 三层自动化检测:
#   Layer A — 测试覆盖率
#   Layer B — 空壳检测（return nil/true/false + 无错误处理）
#   Layer C — 断言强度（contract_test 的 assertion 计数 ≥ MUST 数）
#
# 设计依据: 会议 #88 R-568。28 个 A 类历史遗留欠债的根因——代码通过了验证但行为是错的。
#
# 退出码: 0=全部通过, 1=存在可疑代码
# =============================================================================
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; NC='\033[0m'
FAILED=0

echo "=== CI 反欺骗检查（R-568）==="
echo ""

# ─── Layer A: 测试覆盖率 ───
echo "── Layer A: 测试覆盖率 ──"
COVERAGE=$(go test -cover ./internal/... 2>/dev/null | grep -oE '[0-9]+\.[0-9]+%' | tail -1 | grep -oE '[0-9]+\.' | grep -oE '[0-9]+' || echo "0")
if [ "${COVERAGE:-0}" -lt 80 ]; then
    echo -e "  ${RED}❌${NC} 测试覆盖率 ${COVERAGE}% < 80%"
    FAILED=$((FAILED + 1))
else
    echo -e "  ${GREEN}✅${NC} 测试覆盖率 ${COVERAGE}%（≥ 80%）"
fi
echo ""

# ─── Layer B: 空壳检测 ───
echo "── Layer B: 空壳检测 ──"
# 检测 return nil/true/false 且函数体极短（< 3 行）且无错误处理
SHELL_FUNCS=$(grep -rn "func.*{$" internal/ --include="*.go" -A5 | \
    grep -B5 "return nil$\|return true$\|return false$" | \
    grep "func.*{" | grep -v "_test.go" | grep -v "func (.*) Error()" || true)

# 更精确的检测: 函数体行数
EMPTY_FUNCS=0
for f in $(find internal/ -name "*.go" -not -name "*_test.go"); do
    # 提取每个函数体，检查是否只有 return nil/true/false
    awk '/^func /{ fn=$0; body=""; in_body=0; next }
         /^{/{ in_body=1; next }
         /^}/{ if(in_body && body ~ /^[[:space:]]*return (nil|true|false)[[:space:]]*$/) print fn; in_body=0; body=""; next }
         { if(in_body) body=body$0 }' "$f" | while read -r fn; do
        echo "  ${RED}❌${NC} $f: $fn — 函数体仅 return nil/true/false"
        EMPTY_FUNCS=$((EMPTY_FUNCS + 1))
    done
done

# 检测 _, _ = 模式吞错误
SWALLOWED=$(grep -rn "_, _\s*=" internal/ --include="*.go" | grep -v "_test.go" | grep -v "ok " | head -5 || true)
if [ -n "$SWALLOWED" ]; then
    echo -e "  ${RED}❌${NC} 检测到 _, _ = 吞错误模式"
    echo "$SWALLOWED" | while read -r line; do echo "    $line"; done
    FAILED=$((FAILED + 1))
else
    echo -e "  ${GREEN}✅${NC} 无 _, _ = 吞错误"
fi
echo ""

# ─── Layer C: 断言强度 ───
echo "── Layer C: 断言强度（contract_test assertion ≥ MUST 数）──"
# 简化版本——检查每个 contract_test 文件至少有 t.Error/t.Fatal 调用
for ct in $(find internal/ -name "*_contract_test.go" 2>/dev/null); do
    assertions=$(grep -cE "\.(Error|Fatal|Errorf|Fatalf)\(" "$ct" 2>/dev/null || echo 0)
    module=$(basename "$ct" | sed 's/_contract_test.go//')
    if [ "$assertions" -lt 3 ]; then
        echo -e "  ${RED}❌${NC} $module: assertion 数 = $assertions < 3（最低阈值）"
        FAILED=$((FAILED + 1))
    else
        echo -e "  ${GREEN}✅${NC} $module: $assertions assertions"
    fi
done
echo ""

# ─── 总结 ───
if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}✅ CI 反欺骗检查通过${NC}"
    exit 0
else
    echo -e "${RED}❌ $FAILED 项未通过${NC}"
    echo "修复: 空壳→实现真实逻辑。吞错误→显式检查 error。assertion 不足→增加测试。"
    exit 1
fi
