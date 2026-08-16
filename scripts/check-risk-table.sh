#!/bin/bash
# =============================================================================
# GoalOS 风险表生成式验证（D44 R-1179 / S'-25 R-1268）
#
# 单一数据源: scripts/risk-formula.yaml（两层 schema:
#   formula=权重+阈值, dimension-matrix=每命令维度赋值矩阵）
# 本脚本只读 risk-formula.yaml 重算映射表并与 05软件架构文档.md 的
# capability_risk_map 比对——禁内嵌第二份维度表（维度变更只改 yaml）。
# 校验项:
#   1. yaml 可解析（字段/枚举/阈值完整性）
#   2. 维度值必须落在公式枚举内（枚举外分值=数据错误, R-1179）
#   3. 每命令重算 sum→R 级，与 05 表 # R<level> 标注一致
#   4. 每命令四个维度值与 05 表逐维一致
#   5. 命令集合双向一致（yaml 有 05 无 / 05 有 yaml 无 均失败）
#
# R-1052 三布局自适应:
#   工作区布局   : CWD=workspace 根（开发文档/ + GoalOS/）
#   repo-only 布局: CWD=GoalOS/ 仓库内（无 开发文档/）——显式降级 SKIP 不追溯
#   CWD-relative : 按脚本自身位置推导（从 workspace 根运行检查脚本）
# 退出码: 0=全部通过/降级跳过, 1=存在漂移
# 最后更新: 2026-08-13（会议 #200 S'-25 R-1268——风险表生成式验证接线）
# =============================================================================
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)

# ── 布局探测（只依据目录结构；yaml 缺失由下游红色报错而非降级 SKIP）──
WS=""
if [ -d "$SCRIPT_DIR/../../开发文档" ]; then
  # 脚本位于 workspace/GoalOS/scripts/，向上两级是 workspace 根
  WS="$(cd "$SCRIPT_DIR/../.." && pwd)"
elif [ -d "开发文档" ]; then
  WS="$PWD"
fi

if [ -z "$WS" ]; then
  echo -e "${YELLOW}⚠️${NC} 未定位工作区布局（无 开发文档/）——repo-only 布局显式降级: ⏭️ SKIP 不追溯"
  exit 0
fi

YAML="$WS/GoalOS/scripts/risk-formula.yaml"
DOC="$WS/开发文档/05软件架构文档.md"
FAILED=0

echo "=== 风险表生成式验证（D44 R-1179 / S'-25 R-1268）==="
echo ""

# ═══ 1. 数据源解析 ═══
if [ ! -f "$YAML" ]; then
  echo -e "${RED}❌${NC} 数据源缺失: GoalOS/scripts/risk-formula.yaml（S'-25 单一数据源）"
  echo "修复: 创建 risk-formula.yaml（formula 权重+阈值 + dimension-matrix 9 命令维度矩阵）"
  exit 1
fi

# 阈值表: - {level: R0, min: 0, max: 0}
THR=$(grep -E '^    - \{level: R[0-5], min: [0-9]+, max: [0-9]+\}' "$YAML" || true)
if [ -z "$THR" ]; then
  echo -e "${RED}❌${NC} risk-formula.yaml: 阈值表缺失或格式错误（需 formula.thresholds 六行 level/min/max）"
  exit 1
fi
# 维度枚举: dimension_values 块
ENUM_LINE=$(grep -A4 '^  dimension_values:' "$YAML" | grep -E '^    [a-z]+: \[' || true)
if [ -z "$ENUM_LINE" ]; then
  echo -e "${RED}❌${NC} risk-formula.yaml: dimension_values 枚举缺失（destructiveness/external/privilege/reversibility）"
  exit 1
fi
# 命令矩阵: dimension-matrix 块（行首可含缩进）
MATRIX=$(awk '/^dimension-matrix:/{f=1;next} f&&/^[[:space:]]*[a-z_.]+: *\{/{gsub(/^[[:space:]]*/,""); print}' "$YAML" || true)
if [ -z "$MATRIX" ]; then
  echo -e "${RED}❌${NC} risk-formula.yaml: dimension-matrix 缺失（9 命令维度赋值）"
  exit 1
fi

# 权重必须全 1（D44 裁决结果）
W=$(awk '/^  weights:/{f=1;next} f&&/^    [a-z]+: [0-9]+/{print; if(++n==4) exit}' "$YAML" || true)
if ! echo "$W" | grep -qE 'destructiveness: 1|external: 1|privilege: 1|reversibility: 1'; then
  echo -e "${RED}❌${NC} risk-formula.yaml: 权重非全 1（D44 裁决目标态: 权重全 1 求和）"
  FAILED=$((FAILED+1))
fi

# 阈值重算函数: sum → R 级（awk 读阈值表）
level_of() {
  awk -v sum="$1" '
    /^    - \{level: R[0-5], min: [0-9]+, max: [0-9]+\}/ {
      l=$0; gsub(/.*level: /,"",l); gsub(/,.*/,"",l)
      mn=$0; gsub(/.*min: /,"",mn); gsub(/,.*/,"",mn)
      mx=$0; gsub(/.*max: /,"",mx); gsub(/[^0-9].*/,"",mx)
      if (sum>=mn && sum<=mx) print l
    }' "$YAML" | head -1
}

# 枚举校验 + 命令矩阵解析
check_enum() { # $1=维度名 $2=值 $3=命令
  local allowed
  allowed=$(echo "$ENUM_LINE" | grep -E "^    $1: \[[0-9, ]+\]" | grep -oE '[0-9]+' | tr '\n' ' ')
  if ! echo " $allowed " | grep -q " $2 "; then
    echo -e "  ${RED}❌${NC} $3.$1=$2: 枚举外分值=数据错误（R-1179，公式枚举 $(echo $allowed | xargs)）"
    return 1
  fi
  return 0
}

echo "── 1. yaml 重算比对 ──"
# 05 表提取（capability_risk_map 代码块内命令行）
TABLE=$(awk '/^capability_risk_map:/{f=1;next} f&&/^```/{exit} f&&/^[[:space:]]*[a-z_.]+: *\{/{gsub(/^[[:space:]]*/,""); print}' "$DOC" || true)
if [ -z "$TABLE" ]; then
  echo -e "${RED}❌${NC} 05: capability_risk_map 表未提取到"
  FAILED=$((FAILED+1))
fi
# 两侧落临时文件——后续 grep 读文件（无 echo|grep 管道竞态）
MF=$(mktemp); TF=$(mktemp)
echo "$MATRIX" > "$MF"; echo "$TABLE" > "$TF"

while IFS= read -r row; do
  cmd=$(echo "$row" | sed -E 's/^([a-z_.]+):.*/\1/')
  vals=$(echo "$row" | sed -E 's/.*\{([^}]*)\}.*/\1/')
  d=$(echo "$vals" | sed -E 's/.*destructiveness: *([0-9]+).*/\1/')
  e=$(echo "$vals" | sed -E 's/.*external: *([0-9]+).*/\1/')
  p=$(echo "$vals" | sed -E 's/.*privilege: *([0-9]+).*/\1/')
  r=$(echo "$vals" | sed -E 's/.*reversibility: *([0-9]+).*/\1/')
  # 枚举校验
  enum_fail=0
  check_enum "destructiveness" "$d" "$cmd" >/dev/null || enum_fail=1
  check_enum "external" "$e" "$cmd" >/dev/null || enum_fail=1
  check_enum "privilege" "$p" "$cmd" >/dev/null || enum_fail=1
  check_enum "reversibility" "$r" "$cmd" >/dev/null || enum_fail=1
  [ "$enum_fail" -eq 1 ] && { echo -e "  ${RED}❌${NC} $cmd: 维度值越界"; FAILED=$((FAILED+1)); continue; }
  # 重算
  sum=$((d + e + p + r))
  level=$(level_of "$sum")
  [ -z "$level" ] && { echo -e "  ${RED}❌${NC} $cmd: sum=$sum 无对应阈值级"; FAILED=$((FAILED+1)); continue; }
  # 05 表比对（grep 读临时文件——无管道竞态）
  trow=$(grep -E "^$cmd: *\{" "$TF" || true)
  if [ -z "$trow" ]; then
    echo -e "  ${RED}❌${NC} $cmd: yaml 有命令, 05 表无此命令"; FAILED=$((FAILED+1)); continue
  fi
  tv=$(echo "$trow" | sed -E 's/.*\{([^}]*)\}.*/\1/')
  td=$(echo "$tv" | sed -E 's/.*destructiveness: *([0-9]+).*/\1/')
  te=$(echo "$tv" | sed -E 's/.*external: *([0-9]+).*/\1/')
  tp=$(echo "$tv" | sed -E 's/.*privilege: *([0-9]+).*/\1/')
  tr2=$(echo "$tv" | sed -E 's/.*reversibility: *([0-9]+).*/\1/')
  tlab=$(echo "$trow" | grep -oE '# *R[0-5]' | grep -oE 'R[0-5]' || true)
  if [ "$td" != "$d" ] || [ "$te" != "$e" ] || [ "$tp" != "$p" ] || [ "$tr2" != "$r" ]; then
    echo -e "  ${RED}❌${NC} $cmd: 维度漂移 yaml={$d,$e,$p,$r} vs 05={$td,$te,$tp,$tr2}"
    FAILED=$((FAILED+1)); continue
  fi
  if [ "$tlab" != "$level" ]; then
    echo -e "  ${RED}❌${NC} $cmd: R 标签漂移 yaml 重算=$level vs 05 标注=$tlab (sum=$sum)"
    FAILED=$((FAILED+1)); continue
  fi
done <<< "$MATRIX"

# 反向: 05 有 yaml 无（here-string 保持 FAILED 在当前 shell 可累计; grep 读文件无竞态）
while IFS= read -r trow; do
  cmd=$(echo "$trow" | sed -E 's/^([a-z_.]+):.*/\1/')
  if ! grep -qE "^$cmd: *\{" "$MF"; then
    echo -e "  ${RED}❌${NC} $cmd: 05 表有命令, yaml 无此命令"
    FAILED=$((FAILED+1))
  fi
done <<< "$TABLE"
rm -f "$MF" "$TF"

if [ "$FAILED" -eq 0 ]; then
  echo -e "  ${GREEN}✅${NC} 全部命令维度值+定级与 yaml 重算一致"
fi
echo ""

echo "──────────────────────────────"
echo "  失败合计: $FAILED"
echo ""
[ "$FAILED" -eq 0 ] && echo -e "${GREEN}✅ 风险表生成式验证通过——05 映射表与 risk-formula.yaml 重算一致${NC}" && exit 0
echo -e "${RED}❌ $FAILED 项风险表漂移。D44: 表由公式生成, 变更走公式（改 risk-formula.yaml 后重算）。${NC}"
exit 1
