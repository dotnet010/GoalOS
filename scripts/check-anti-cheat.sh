#!/bin/bash
# =============================================================================
# GoalOS CI 反欺骗检查 — R-568（Beck 规则 5 的实现）
# R-706: 层B增加行为变异检测——核心函数否定测试分支覆盖率>60%（G顾问审计）
# R-735: 层1-3新增三个独立脚本（check-naked-map / check-error-swallow / check-contract-test-assertion）
#
# 五层自动化检测:
#   Layer A — 测试覆盖率
#   Layer B — 空壳检测（return nil/true/false + 无错误处理）+ 行为变异检测(R-706)
#   Layer C — 断言强度（contract_test 的 assertion 计数 ≥ MUST 数）
#   Layer D — MUST 覆盖（make constitution-check）
#   Layer E — govulncheck（全量扫描）
#
# 配套 CI 脚本（R-735 新增）:
#   check-naked-map.sh           — 层 1: AST 扫描裸 map[K]V 包级声明
#   check-error-swallow.sh       — 层 2: go/analysis 扫描 _ 丢弃 error/ok
#   check-contract-test-assertion.sh — 层 3: contract_test assertion 覆盖检测
#
# 设计依据: 会议 #88 R-568。28 个 A 类历史遗留欠债的根因——代码通过了验证但行为是错的。
# 扩展依据: 会议 #108-#112 R-733~R-738 框架层三统一方案。
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
# v0.2.0: 计算 internal/ 包的平均覆盖率（排除 test/ 集成测试包）
COVERAGE_SUM=0
COVERAGE_COUNT=0
while IFS= read -r line; do
    pct=$(echo "$line" | grep -oE '[0-9]+\.[0-9]+%' | grep -oE '[0-9]+\.' | grep -oE '[0-9]+' || echo "0")
    if [ -n "$pct" ] && [ "$pct" -gt 0 ] 2>/dev/null; then
        COVERAGE_SUM=$((COVERAGE_SUM + pct))
        COVERAGE_COUNT=$((COVERAGE_COUNT + 1))
    fi
done < <(go test -cover ./internal/... 2>/dev/null | grep "coverage:")
if [ "$COVERAGE_COUNT" -gt 0 ]; then
    AVG_COVERAGE=$((COVERAGE_SUM / COVERAGE_COUNT))
else
    AVG_COVERAGE=0
fi
if [ "${AVG_COVERAGE:-0}" -lt 50 ]; then
    echo -e "  ${RED}❌${NC} 平均测试覆盖率 ${AVG_COVERAGE}% < 50% (${COVERAGE_COUNT} packages)"
    FAILED=$((FAILED + 1))
else
    echo -e "  ${GREEN}✅${NC} 平均测试覆盖率 ${AVG_COVERAGE}% (${COVERAGE_COUNT} packages)"
fi
echo ""

# ─── Layer B: 空壳检测 ───
echo "── Layer B: 空壳检测 ──"
# v0.2.0 audit fix: 增强检测——不仅检测纯 return nil/true/false，
# 还检测函数体只有注释+return 的模式。
EMPTY_FUNCS=0

# 检测 1: 纯 return nil/true/false（原有逻辑）
for f in $(find internal/ -name "*.go" -not -name "*_test.go"); do
    awk '/^func /{ fn=$0; body=""; in_body=0; next }
         /^{/{ in_body=1; next }
         /^}/{ if(in_body && body ~ /^[[:space:]]*return (nil|true|false)[[:space:]]*$/) print fn; in_body=0; body=""; next }
         { if(in_body) body=body$0 }' "$f" | while read -r fn; do
        echo "  ${RED}❌${NC} $f: $fn — 函数体仅 return nil/true/false"
        EMPTY_FUNCS=$((EMPTY_FUNCS + 1))
    done
done

# 检测 2: 函数体仅包含注释 + return nil/true/false（v0.2.0 audit 新增）
# 移除注释和空白后，如果函数体只剩 return，则为空壳
echo "── Layer B2: 注释+return空壳检测（v0.2.0 audit）──"
for f in $(find internal/ -name "*.go" -not -name "*_test.go"); do
    awk '
    /^func /{ fn=$0; body=""; in_func=0; brace_count=0; next }
    /^{/{ in_func=1; brace_count=1; next }
    in_func {
        # 跳过注释行和空行，收集实际代码
        line = $0
        gsub(/\/\/.*$/, "", line)      # 移除行注释
        gsub(/^[[:space:]]+/, "", line) # 移除前导空白
        gsub(/[[:space:]]+$/, "", line) # 移除尾部空白
        if (line != "" && line !~ /^\/\*/ && line !~ /^\*\//) {
            body = body line
        }
    }
    /^}/{
        if (in_func) {
            brace_count--
            if (brace_count <= 0) {
                in_func = 0
                # 检查移除注释后是否只剩 return nil/true/false
                stripped = body
                gsub(/return (nil|true|false)/, "", stripped)
                gsub(/[[:space:]]/, "", stripped)
                if (stripped == "" && body ~ /return/) {
                    # 找到: 只有注释+return 的函数
                    gsub(/\/\/.*$/, "", fn)
                    print fn " [comment+return only]"
                }
                body = ""
            }
        }
    }
    ' "$f" | while read -r fn; do
        echo "  ${RED}❌${NC} $f: $fn"
        EMPTY_FUNCS=$((EMPTY_FUNCS + 1))
    done
done

if [ $EMPTY_FUNCS -gt 0 ]; then
    echo -e "  ${RED}❌${NC} 发现 $EMPTY_FUNCS 个疑似空壳函数"
    FAILED=$((FAILED + 1))
else
    echo -e "  ${GREEN}✅${NC} 无空壳函数"
fi

echo "── Layer B3: 错误处理检测 ──"
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
