#!/bin/bash
# =============================================================================
# GoalOS CheckWARN 分支治理 — CI 强制（R-1005 D-IMP-01，任务 1.19）
# =============================================================================
#
# 目的:
#   Check 原语 WARN 分支必须显式处理——warn_escalation_threshold 仅统计 Check 原语
#   WARN（R-1232）；验证层 WARN 不计入升级计数（R-1216 双轨）。本脚本扫描
#   internal/scheduler/ 下 Check 原语实现：WARN 分支必须有显式计数器递增或
#   显式不计数注释——静默忽略 WARN=CI FAIL。
#
# 依据: R-1232（warn_escalation_threshold 仅统计 Check 原语 WARN）+ R-1216（双轨）
#       + R-1005（实现补强冲刺 D-IMP-01）
#
# 输出: stdout — 违规位置清单；exit 0=无违规，exit 1=有违规
# =============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

if [ -t 1 ]; then
    RED='\033[0;31m'; GREEN='\033[0;32m'; NC='\033[0m'
else
    RED=''; GREEN=''; NC=''
fi

FAILED=0

echo "=== check-warn-branch: Check 原语 WARN 分支显式处理（R-1005/R-1232）==="

# 扫描 internal/scheduler/ 下 Check 原语相关文件
while IFS= read -r file; do
    # 命中 WARN 分支的行（severity=WARN 或 CheckPerformed WARN）
    HITS=$(grep -nE 'severity.*WARN|CheckPerformed.*WARN|warn.*escalation' "$file" 2>/dev/null || true)
    [ -z "$HITS" ] && continue

    # 检查是否有显式计数器递增（warnCount++/warn_escalation_threshold 引用）
    HAS_COUNTER=$(grep -cE 'warnCount|warn_escalation_threshold|warnings.*append' "$file" 2>/dev/null || true)
    if [ "$HAS_COUNTER" -eq 0 ]; then
        FAILED=$((FAILED + 1))
        echo -e "  ${RED}[FAIL]${NC} ${file#$REPO_DIR/} Check WARN 分支无显式计数器——静默忽略 WARN=CI FAIL（R-1232）:"
        echo "$HITS" | while IFS= read -r line; do echo "    $line"; done
    fi
done < <(find "$REPO_DIR/internal/scheduler" -name '*.go' ! -name '*_test.go' 2>/dev/null)

echo ""
if [ "$FAILED" -gt 0 ]; then
    echo -e "${RED}=== CHECK-WARN-BRANCH CHECK FAILED: $FAILED 违规 ===${NC}" >&2
    exit 1
fi
echo -e "${GREEN}=== CHECK-WARN-BRANCH CHECK ALL GREEN ===${NC}"
exit 0
