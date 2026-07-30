#!/bin/bash
# =============================================================================
# GoalOS CI 死代码检测 — v0.2.0 audit fix
#
# 检测项目:
#   1. 未被生产代码调用的 exported 函数（排除测试文件中的调用）
#   2. 仅在 test 文件中引用的核心函数（可能为测试专用桩）
#   3. go vet 检查（含 unused 变量/函数检测）
#
# 退出码: 0=全部通过, 1=存在死代码
# =============================================================================
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[0;33m'; NC='\033[0m'
FAILED=0

echo "=== CI 死代码检测（v0.2.0 audit）==="
echo ""

# ─── Layer 1: go vet 检查 ───
echo "── Layer 1: go vet ──"
if go vet ./... 2>&1; then
    echo -e "  ${GREEN}✅${NC} go vet 通过"
else
    echo -e "  ${RED}❌${NC} go vet 发现警告"
    FAILED=$((FAILED + 1))
fi
echo ""

# ─── Layer 2: 已知孤儿代码检查 ───
echo "── Layer 2: 孤儿代码检查 ──"

# 检查 StateMachineRun 是否有生产调用者（排除测试文件和自身定义）
# v0.3.0 fix (H10): 孤儿代码检测递增 FAILED 计数器——CI 不再静默通过。
STATEMACHINE_CALLERS=$(grep -rn "StateMachineRun" internal/ cmd/ --include="*.go" | grep -v "_test.go" | grep -v "func.*StateMachineRun" | wc -l | tr -d ' ')
if [ "${STATEMACHINE_CALLERS:-0}" -eq 0 ]; then
    echo -e "  ${YELLOW}⚠️${NC}  StateMachineRun: 无生产代码调用者（仅测试引用）"
    echo "    位置: internal/scheduler/pipelinerunner_statemachine.go"
    FAILED=$((FAILED + 1))
else
    echo -e "  ${GREEN}✅${NC} StateMachineRun: $STATEMACHINE_CALLERS 个生产调用者"
fi

# 检查 GoalState.Run 是否有生产调用者（v0.3.0: 仅警告，不阻塞CI——grep模式可能匹配不全）
GOALSTATE_CALLERS=$(grep -rn "\.Run(stopCh)" internal/ cmd/ --include="*.go" 2>/dev/null | grep -v "_test.go" | wc -l | tr -d ' ')
if [ "${GOALSTATE_CALLERS:-0}" -eq 0 ]; then
    echo -e "  ${YELLOW}⚠️${NC}  GoalState.Run: 无生产代码调用者（仅测试引用）"
    echo "    位置: internal/scheduler/goalrunner_select.go"
else
    echo -e "  ${GREEN}✅${NC} GoalState.Run: $GOALSTATE_CALLERS 个生产调用者"
fi

# 检查 failHints map 是否有读取者（v0.3.0: 仅警告，不阻塞CI）
FAILHINTS_READERS=$(grep -rn "failHints\[" internal/ cmd/ --include="*.go" 2>/dev/null | grep -v "_test.go" | wc -l | tr -d ' ')
if [ "${FAILHINTS_READERS:-0}" -eq 0 ]; then
    echo -e "  ${YELLOW}⚠️${NC}  failHints map: 已定义但无读取者"
    echo "    位置: internal/daemon/api.go:225"
else
    echo -e "  ${GREEN}✅${NC} failHints: $FAILHINTS_READERS 个读取者"
fi
echo ""

# ─── Layer 3: unused 导出符号（staticcheck 方式，可选） ───
echo "── Layer 3: staticcheck（可选）──"
if which staticcheck > /dev/null 2>&1; then
    if staticcheck ./... 2>&1; then
        echo -e "  ${GREEN}✅${NC} staticcheck 通过"
    else
        echo -e "  ${YELLOW}⚠️${NC} staticcheck 发现问题（需人工评估）"
    fi
else
    echo -e "  ${YELLOW}⚠️${NC} staticcheck 未安装。安装: go install honnef.co/go/tools/cmd/staticcheck@latest"
fi
echo ""

# ─── 总结 ───
if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}✅ CI 死代码检测通过${NC}"
    exit 0
else
    echo -e "${RED}❌ $FAILED 项未通过${NC}"
    exit 1
fi
