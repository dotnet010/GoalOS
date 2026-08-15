#!/bin/bash
# =============================================================================
# GoalOS 沙箱 import 治理 — CI 强制（R-953，任务 1.12）
# =============================================================================
#
# 目的:
#   仅 internal/pluginrunner 可 import internal/sandbox——Sandbox 是执行环境构造层，
#   不管理插件进程生命周期（R-1111 职责边界）；治理不变量（R-906a）：sandbox.Execute
#   的调用点必须位于 Governance 治理闸门之后，调用链=PipelineRunner→五引擎→
#   PluginRunner→sandbox.Execute。其他模块直接 import sandbox=绕过治理闸门。
#
# 检查规则:
#   [MUST] 扫描 internal/ 与 cmd/ 下所有 .go 文件（排除 _test.go）
#   [MUST] 凡 import "github.com/goalos/goalos/internal/sandbox" 的文件，其包路径
#          必须属于 internal/pluginrunner（含子包 ipc/egressproxy）
#   [MUST] 其他模块（scheduler/governance/missionengine/daemon/...）import sandbox=FAIL
#
# 依据: R-953（CI 治理强制）+ R-1111（组件边界契约）+ R-906a（治理不变量）
#
# 输出: stdout — 违规文件清单；exit 0=无违规，exit 1=有违规
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

echo "=== check-sandbox-import: 仅 pluginrunner 可 import internal/sandbox（R-953）==="

# 扫描 internal/ 与 cmd/ 下所有 .go 文件（排除 _test.go）
while IFS= read -r file; do
    # 命中 import internal/sandbox 的行
    HITS=$(grep -nE '"github.com/goalos/goalos/internal/sandbox"' "$file" 2>/dev/null || true)
    [ -z "$HITS" ] && continue

    # 允许路径：internal/pluginrunner/**（含子包）
    case "$file" in
        "$REPO_DIR/internal/pluginrunner/"*)
            # 合法——pluginrunner 是 sandbox 的唯一调用方
            ;;
        *)
            FAILED=$((FAILED + 1))
            echo -e "  ${RED}[FAIL]${NC} ${file#$REPO_DIR/} import internal/sandbox——仅 pluginrunner 可调用（R-953/R-906a 治理不变量）:"
            echo "$HITS" | while IFS= read -r line; do echo "    $line"; done
            ;;
    esac
done < <(find "$REPO_DIR/internal" "$REPO_DIR/cmd" -name '*.go' ! -name '*_test.go' 2>/dev/null)

echo ""
if [ "$FAILED" -gt 0 ]; then
    echo -e "${RED}=== SANDBOX IMPORT CHECK FAILED: $FAILED 违规 ===${NC}" >&2
    exit 1
fi
echo -e "${GREEN}=== SANDBOX IMPORT CHECK ALL GREEN ===${NC}"
exit 0
