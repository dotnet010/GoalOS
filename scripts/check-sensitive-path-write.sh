#!/bin/bash
# =============================================================================
# check-sensitive-path-write.sh——测试夹具敏感路径写入机检（会议 #272 提案落地）
#
# 事故史（2026-09-06 实证）：WinAC Boundary②测试直接 os.WriteFile(真实
# ~/.ssh/config)——毁损用户 SSH 配置（goalos-test 登录条目丢失）。
# 教训：测试=可能跑在任何人的机器上——夹具永不写真实用户既有文件。
#
# 规则（三条件同文件同满=红——反证迭代两轮实证：单行模式漏过变量间接形态，
# 纯构造匹配误伤分类断言）：
#   ①文件构造敏感目录（".ssh"/".aws"/".gnupg" 字面量）
#   ②文件出现敏感文件终段字面量（"config"/"id_*"/"credentials"/".gitconfig"/
#     ".netrc"）且该行无豁免标记（goalos- 夹具名 或 safefixture: 注释）
#   ③文件含写入动词（os.WriteFile/Create/MkdirAll/Remove/RemoveAll）
# 纯分类/纯读取（无写入动词）=合法通过；goalos- 专用夹具名=合法新建。
#
# 退出码: 0=无违规, 1=存在违规
# =============================================================================
set -uo pipefail

RED='\033[0;31m'; GREEN='\033[0m'; NC='\033[0m'
FAILED=0

echo "=== check-sensitive-path-write: 测试夹具敏感路径写入扫描（三段判定） ==="

FILES=$(grep -rlE '"\.(ssh|aws|gnupg)"' --include="*_test.go" internal/ cmd/ pkg/ test/ 2>/dev/null || true)
for f in $FILES; do
	# 条件②：敏感终段行（无豁免标记）
	SUS=$(grep -nE '"(config|id_[a-zA-Z0-9_]*|credentials|\.gitconfig|\.netrc|known_hosts)"' "$f" | grep -v "goalos-" | grep -v "safefixture:" || true)
	[ -z "$SUS" ] && continue
	# 条件③：写入动词在场
	if grep -qE 'os\.(WriteFile|Create|MkdirAll|Remove|RemoveAll)\(' "$f"; then
		echo -e "${RED}❌ FAIL: $f${NC}"
		echo "$SUS" | head -5
		echo "  → 同文件含写入动词+敏感终段构造——夹具须用 goalos- 专用名或 safefixture: 注释豁免"
		FAILED=1
	fi
done

if [ "$FAILED" -eq 0 ]; then
    echo -e "${GREEN}✅ 通过——测试夹具零触碰真实敏感条目${NC}"
fi
exit $FAILED
