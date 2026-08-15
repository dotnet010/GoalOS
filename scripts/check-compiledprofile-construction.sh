#!/bin/bash
# =============================================================================
# GoalOS CompiledProfile 构造治理 — CI 强制（R-1036，任务 1.25）
# =============================================================================
#
# 目的:
#   CompiledProfile 必须经 Compile() 产出——直接构造的零值在 Execute 入口被
#   Fatal fail-closed 拒绝（R-1106 零值非法化）。本脚本扫描 internal/sandbox 包内
#   CompiledProfile{ 复合字面量——仅 Compile() 内允许直接构造。
#
# 检查规则:
#   [MUST] 扫描 internal/sandbox/ 下所有 .go 文件（排除 _test.go）
#   [MUST] 凡 CompiledProfile{ 复合字面量出现在 Compile() 之外的函数=FAIL
#
# 依据: R-1106（零值非法化）+ R-1036（构造治理 CI）
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

echo "=== check-compiledprofile-construction: CompiledProfile 仅 Compile() 内构造（R-1036）==="

# 扫描 internal/sandbox/ 下所有 .go 文件（排除 _test.go）
while IFS= read -r file; do
    # 命中 CompiledProfile{ 复合字面量的行
    HITS=$(grep -nE 'CompiledProfile\s*\{' "$file" 2>/dev/null || true)
    [ -z "$HITS" ] && continue

    # 检查每个命中是否在 Compile() 函数内
    while IFS= read -r line; do
        lineno="${line%%:*}"
        # 提取该行所在的函数名（向上找最近的 func 行）
        func_name=$(sed -n "1,${lineno}p" "$file" | grep -E '^func ' | tail -1 | sed -E 's/^func (\([^)]*\) )?([A-Za-z_][A-Za-z0-9_]*).*/\2/')
        if [ "$func_name" != "Compile" ]; then
            FAILED=$((FAILED + 1))
            echo -e "  ${RED}[FAIL]${NC} ${file#$REPO_DIR/}:$lineno CompiledProfile{ 出现在 $func_name()——仅 Compile() 内允许（R-1106 零值非法化）"
        fi
    done <<< "$HITS"
done < <(find "$REPO_DIR/internal/sandbox" -name '*.go' ! -name '*_test.go' 2>/dev/null)

echo ""
if [ "$FAILED" -gt 0 ]; then
    echo -e "${RED}=== COMPILEDPROFILE CONSTRUCTION CHECK FAILED: $FAILED 违规 ===${NC}" >&2
    exit 1
fi
echo -e "${GREEN}=== COMPILEDPROFILE CONSTRUCTION CHECK ALL GREEN ===${NC}"
exit 0
