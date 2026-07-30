#!/bin/bash
# check-contract-test-assertion.sh — CI 反欺骗层：contract_test assertion 覆盖检测
# 依据: R-735 CI 三层强制 — 层 3
# 规则: contract_test 文件的 t.Error/t.Fatal 调用数 ≥ 模块 MUST 数
#       assertion 弱于 MUST（如 assert.NotNil 替代精确验证）→ WARN

set -euo pipefail

ROOT_DIR="${1:-.}"
FAILED=0
WARNINGS=0

echo "=== check-contract-test-assertion: 扫描 contract_test assertion 覆盖 ==="

# 查找所有 contract_test.go 文件
while IFS= read -r file; do
    MODULE_NAME=$(basename "$file" _contract_test.go)

    # 统计 assertion 调用数（t.Error/t.Fatal/t.Errorf/t.Fatalf）
    ASSERTION_COUNT=$(grep -cE '\.(Error|Fatal|Errorf|Fatalf)\(' "$file" 2>/dev/null || true)
    ASSERTION_COUNT=${ASSERTION_COUNT:-0}
    ASSERTION_COUNT=${ASSERTION_COUNT//[^0-9]/}

    # 统计断言库调用（testify: assert./require.）
    TESTIFY_COUNT=$(grep -cE '(assert|require)\.(Equal|NotEqual|Nil|NotNil|True|False|Error|NoError|Contains|NotContains|Empty|NotEmpty|Less|Greater|Len)\(' "$file" 2>/dev/null || true)
    TESTIFY_COUNT=${TESTIFY_COUNT:-0}
    TESTIFY_COUNT=${TESTIFY_COUNT//[^0-9]/}

    TOTAL_ASSERTIONS=$((ASSERTION_COUNT + TESTIFY_COUNT))

    # 统计 assert.NotNil 使用次数——NotNil 不算精确 assertion
    WEAK_NOTNIL=$(grep -cE '(assert|require)\.NotNil\(' "$file" 2>/dev/null || true)
    WEAK_NOTNIL=${WEAK_NOTNIL:-0}
    WEAK_NOTNIL=${WEAK_NOTNIL//[^0-9]/}

    # 获取对应模块的 MUST 数量——从架构文档或模块契约定义中提取
    MUST_COUNT=$(grep -oE 'MUST[: ]+[0-9]+' "$file" 2>/dev/null | grep -oE '[0-9]+' | head -1 || true)
    MUST_COUNT=${MUST_COUNT:-0}
    MUST_COUNT=${MUST_COUNT//[^0-9]/}
    if [ "$MUST_COUNT" -eq 0 ]; then
        # 如果注释中没有标注，尝试从文件名推断（常见模块的 MUST 数）
        case "$MODULE_NAME" in
            eventbus)       MUST_COUNT=3 ;;
            pipeline)       MUST_COUNT=4 ;;
            goalrunner)     MUST_COUNT=3 ;;
            scheduler)      MUST_COUNT=3 ;;
            governance)     MUST_COUNT=15 ;;
            pluginrunner)   MUST_COUNT=3 ;;
            contextengine)  MUST_COUNT=3 ;;
            statestore)     MUST_COUNT=3 ;;
            security*|token*) MUST_COUNT=3 ;;
            ipc*|fd3*)      MUST_COUNT=3 ;;
            hmac*)          MUST_COUNT=3 ;;
            macos*)         MUST_COUNT=2 ;;
            inputvalidator*) MUST_COUNT=5 ;;
            safemap*)       MUST_COUNT=3 ;;
            *)              MUST_COUNT=0 ;;
        esac
    fi

    echo ""
    echo "模块: $MODULE_NAME | 文件: $file"
    echo "  Assertion 总数: $TOTAL_ASSERTIONS (t.Error/assert: $ASSERTION_COUNT + testify: $TESTIFY_COUNT)"
    if [ "$MUST_COUNT" -gt 0 ]; then
        echo "  MUST 数: $MUST_COUNT"
    else
        echo "  MUST 数: 未标注（跳过 assertion 数量检查）"
    fi
    echo "  Weak NotNil: $WEAK_NOTNIL"

    # 检查 assertion 数量 ≥ MUST 数量
    if [ "$MUST_COUNT" -gt 0 ] && [ "$TOTAL_ASSERTIONS" -lt "$MUST_COUNT" ]; then
        echo "  ❌ FAIL: assertion 数 ($TOTAL_ASSERTIONS) < MUST 数 ($MUST_COUNT)"
        FAILED=1
    fi

    # 检查弱 assertion——NotNil 数量超过总 assertion 的 30%
    if [ "$TOTAL_ASSERTIONS" -gt 0 ]; then
        WEAK_RATIO=$(( (WEAK_NOTNIL * 100) / TOTAL_ASSERTIONS ))
        if [ "$WEAK_RATIO" -gt 30 ]; then
            echo "  ⚠️  WARN: assert.NotNil 占比 ${WEAK_RATIO}% (>30%)——可能存在弱 assertion"
            WARNINGS=1
        fi
    fi

    # 检查空壳——任何 contract_test 文件总 assertion < 3
    if [ "$TOTAL_ASSERTIONS" -lt 3 ]; then
        echo "  ❌ FAIL: 总 assertion 数 ($TOTAL_ASSERTIONS) < 3——疑似空壳 contract_test"
        FAILED=1
    fi
done < <(find "$ROOT_DIR" -name "*_contract_test.go" \
    ! -path "*/vendor/*" \
    ! -path "*/.git/*" 2>/dev/null)

if [ -z "$(find "$ROOT_DIR" -name '*_contract_test.go' ! -path '*/vendor/*' ! -path '*/.git/*' 2>/dev/null)" ]; then
    echo ""
    echo "⚠️  WARN: 未找到任何 contract_test.go 文件"
    echo "  每个模块必须包含 contract_test.go——验证 MUST/MUST_NOT 行为契约"
    WARNINGS=1
fi

echo ""
if [ "$FAILED" -eq 1 ]; then
    echo "=== check-contract-test-assertion: ❌ FAILED ==="
    echo "规则: contract_test assertion 数 ≥ 模块 MUST 数。虚假的绿色测试比没有测试更危险。R-735 层 3。"
    exit 1
elif [ "$WARNINGS" -eq 1 ]; then
    echo "=== check-contract-test-assertion: ⚠️  FAILED — weak assertions detected ==="
    echo "规则: 每个 MUST 必须精确翻译为 assertion。弱 assertion（assert.NotNil/assert.True）不满足。v0.3.0 fix (H11): 警告现在阻塞 CI。"
    exit 1
else
    echo "=== check-contract-test-assertion: ✅ PASSED ==="
    exit 0
fi
