#!/bin/bash
# =============================================================================
# GoalOS 错误码单一数据源验证（R-1022/R-1118 / R-1267 S''-24② / R-1300 F-18）
#
# 唯一维护侧: scripts/error-codes-source.yaml（事件字段映射 32 行:
#   error_type 14 + failure_type 7 + reason 10 + reject_reason 1）。
# 比对项:
#   1. yaml ↔ 09 §2.5 表 双向（值/投影域/投影严重级一致, 双向比对,
#      R-1118 顺序纪律: 先改 yaml 再改 09 表再改 07）
#   2. error_type/failure_type: yaml ↔ 07 内联枚举 双向（集合相等）
#   3. reason/reject_reason: yaml ⊆ 07 内联枚举 单向（投影子集——
#      07 的 reason 字段跨多个事件, 其余事件的枚举值不进 09 投影表）
#   4. 跨字段语义一致性: 同名枚举值跨字段 domain/severity 必须一致
#      （timeout=timeout、crash=crash——09 §2.5 尾部条款）
#   5. pkg/errors/codes.go: 显式 SKIP（R-1159 §3.2——错误码常量随实现
#      阶段落地后接线, 当前实现阶段未开始）
#
# R-1052 三布局自适应:
#   工作区布局   : CWD=workspace 根（开发文档/ + GoalOS/）
#   repo-only 布局: CWD=GoalOS/ 仓库内（无 开发文档/）——显式降级 SKIP 不追溯
# 退出码: 0=全部通过/降级跳过, 1=存在漂移
# 最后更新: 2026-08-13（会议 #200 S''-24 R-1267——单一数据源机检接线;
#            会议 #201 F-18 R-1300——唯一维护侧声明改写）
# =============================================================================
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)

# ── 布局探测（只依据目录结构；yaml 缺失由下游红色报错而非降级 SKIP）──
WS=""
if [ -d "$SCRIPT_DIR/../../开发文档" ]; then
  WS="$(cd "$SCRIPT_DIR/../.." && pwd)"
elif [ -d "开发文档" ]; then
  WS="$PWD"
fi

if [ -z "$WS" ]; then
  echo -e "${YELLOW}⚠️${NC} 未定位工作区布局（无 开发文档/）——repo-only 布局显式降级: ⏭️ SKIP 不追溯"
  exit 0
fi

YAML="$WS/GoalOS/scripts/error-codes-source.yaml"
DOC09="$WS/开发文档/09错误码知识库规范.md"
DOC07="$WS/开发文档/07事件注册表.md"
DOC01="$WS/开发文档/01-prd产品需求文档.md"
FAILED=0

echo "=== 错误码单一数据源验证（R-1022/R-1118/R-1267/R-1300/R-1395）==="
echo ""

# ═══ 0. 数据源存在性 ═══
if [ ! -f "$YAML" ]; then
  echo -e "${RED}❌${NC} 数据源缺失: GoalOS/scripts/error-codes-source.yaml（R-1300 唯一维护侧）"
  echo "修复: 创建 error-codes-source.yaml（32 行 event_field_mapping: field/value/domain/severity）"
  exit 1
fi

# ═══ 1. yaml 解析 ═══
YROWS=$(awk '/^event_field_mapping:/{f=1;next} f&&/^  - \{/{gsub(/^[[:space:]]*- /,""); print}' "$YAML" || true)
if [ -z "$YROWS" ]; then
  echo -e "${RED}❌${NC} error-codes-source.yaml: event_field_mapping 行缺失或格式错误"
  exit 1
fi
# 每行必须含 field/value/domain/severity 四键
bad_rows=$(echo "$YROWS" | grep -vE '^\{field: (error_type|failure_type|reason|reject_reason), value: [a-z_]+( N actions failed)?, domain: ([A-Z—]+|—), severity: ([RWFI—]+|—)\}' || true)
if [ -n "$bad_rows" ]; then
  echo -e "${RED}❌${NC} error-codes-source.yaml: 行格式非法:"
  echo "$bad_rows" | head -5 | sed 's/^/    /'
  FAILED=$((FAILED+1))
fi

# 提取 yaml 元组: value|field|domain|severity
YVAL() { echo "$YROWS" | sed -E 's/^\{field: ([a-z_]+), value: ([^,]+), domain: ([^,]+), severity: ([^}]+)\}.*/\2|\1|\3|\4/'; }

# ═══ 2. 09 §2.5 表双向比对 ═══
echo "── 1. yaml ↔ 09 §2.5 表双向 ──"
# 09 表行: | field | value | domain | severity | note |
T09=$(awk '/^## 2.5 /{f=1;next} f&&/^# 3\./{exit} f&&/^\| (error_type|failure_type|reason|reject_reason) \|/{print}' "$DOC09" || true)
if [ -z "$T09" ]; then
  echo -e "${RED}❌${NC} 09 §2.5: 映射表未提取到"
  FAILED=$((FAILED+1))
fi
T09VAL() { echo "$T09" | awk -F'|' '{gsub(/^ +| +$/,"",$2); gsub(/^ +| +$/,"",$3); gsub(/^ +| +$/,"",$4); gsub(/^ +| +$/,"",$5); print $3"|"$2"|"$4"|"$5}'; }

# 两侧元组落临时文件后比对——grep 读文件（无 echo|grep 管道竞态、无 subshell 陷阱）
T09F=$(mktemp); YF=$(mktemp)
T09VAL > "$T09F"; YVAL > "$YF"
# yaml 有 09 无
while IFS= read -r yrow; do
  if ! grep -qxF "$yrow" "$T09F"; then
    echo -e "  ${RED}❌${NC} yaml 有, 09 §2.5 表无: $yrow（R-1118: 先改 yaml 再改 09 表）"
    FAILED=$((FAILED+1))
  fi
done < "$YF"
# 09 有 yaml 无
while IFS= read -r trow; do
  if ! grep -qxF "$trow" "$YF"; then
    echo -e "  ${RED}❌${NC} 09 §2.5 表有, yaml 无: $trow（表由脚本生成——回填 yaml）"
    FAILED=$((FAILED+1))
  fi
done < "$T09F"
rm -f "$T09F" "$YF"

# ═══ 3. 07 内联枚举比对 ═══
echo "── 2. yaml ↔ 07 内联枚举 ──"
# 07 枚举提取: 字段行（仅 reason/reject_reason/error_type/failure_type, 排除 | null 与子字段）
E07_ET=$(grep -E '^ *error_type: string.*"' "$DOC07" 2>/dev/null | grep -oE '"[a-z_]+"' | tr -d '"' | sort -u || true)
E07_FT=$(grep -E '^ *failure_type: string.*"' "$DOC07" 2>/dev/null | grep -oE '"[a-z_]+"' | tr -d '"' | sort -u || true)
E07_RR=$(grep -E '^ *reject_reason: string.*"' "$DOC07" 2>/dev/null | grep -oE '"[a-z_]+"' | tr -d '"' | sort -u || true)
E07_RS=$(grep -E '^ *reason: string.*"' "$DOC07" 2>/dev/null | grep -oE '"[a-z_ ]+"' | tr -d '"' | sort -u || true)

Y_ET=$(echo "$YROWS" | grep -E '^\{field: error_type,' | sed -E 's/.*value: ([^,]+),.*/\1/' | sort -u)
Y_FT=$(echo "$YROWS" | grep -E '^\{field: failure_type,' | sed -E 's/.*value: ([^,]+),.*/\1/' | sort -u)
Y_RR=$(echo "$YROWS" | grep -E '^\{field: reject_reason,' | sed -E 's/.*value: ([^,]+),.*/\1/' | sort -u)
Y_RS=$(echo "$YROWS" | grep -E '^\{field: reason,' | sed -E 's/.*value: ([^,]+),.*/\1/' | sort -u)

# 双向: error_type / failure_type（R-1118 集合相等）
# 单向: reason / reject_reason（yaml ⊆ 07——投影子集, 07 的 reason 跨多事件）
# 注意: cut -d: 对多行变量逐行切分会产出垃圾——此处用 case 直接引用变量
for name in error_type failure_type reason reject_reason; do
  case $name in
    error_type)    yv="$Y_ET"; ev="$E07_ET"; two_way=1 ;;
    failure_type)  yv="$Y_FT"; ev="$E07_FT"; two_way=1 ;;
    reason)        yv="$Y_RS"; ev="$E07_RS"; two_way=0 ;;
    reject_reason) yv="$Y_RR"; ev="$E07_RR"; two_way=0 ;;
  esac
  missing_in_07=$(comm -23 <(echo "$yv") <(echo "$ev") || true)
  if [ -n "$missing_in_07" ]; then
    echo -e "  ${RED}❌${NC} $name: yaml 有, 07 内联枚举无: $(echo $missing_in_07 | tr '\n' ' ')"
    FAILED=$((FAILED+1))
  fi
  if [ "$two_way" -eq 1 ]; then
    missing_in_yaml=$(comm -13 <(echo "$yv") <(echo "$ev") || true)
    if [ -n "$missing_in_yaml" ]; then
      echo -e "  ${RED}❌${NC} $name: 07 内联枚举有, yaml 无: $(echo $missing_in_yaml | tr '\n' ' ')"
      FAILED=$((FAILED+1))
    fi
  fi
done

# ═══ 4. 跨字段语义一致性（09 §2.5 尾部条款）═══
echo "── 3. 跨字段一致性（同名值 domain/severity 一致）──"
CROSS_BAD=$(YVAL | sort | awk -F'|' '
  { v=$1; d=$3; s=$4 }
  v==prev_v && (d!=prev_d || s!=prev_s) { print v"|"prev_d"|"prev_s" vs "v"|"d"|"s }
  { prev_v=v; prev_d=d; prev_s=s }')
while IFS= read -r bad; do
  [ -z "$bad" ] && continue
  echo -e "  ${RED}❌${NC} 跨字段不一致: $bad"
  FAILED=$((FAILED+1))
done <<< "$CROSS_BAD"

# ═══ 5. codes.go 显式 SKIP ═══
echo "── 4. pkg/errors/codes.go 常量比对 ──"
echo -e "  ${YELLOW}⚠️${NC} 显式 SKIP（R-1159 §3.2——错误码常量随实现阶段落地后接线; 当前为文档/CI 阶段）"

# ═══ 6. 01 §17.9 单向指针校验（R-1395: 01 引用的枚举集合 ⊆ 09 §2 条目集合）═══
echo "── 5. 01 §17.9 ↔ 09 §2 单向指针 ──"
# Set A: 01 §17.9 failHints 表"09 条目号（单向指针）"列引用的枚举名（"09 §2：<NAME>" 形态）
# Set B: 09 §2 条目标题中的 AgentErrorCode=<NAME> 标注
A01=$(grep -E '^\| [A-Z_]+ \| 09 §2：' "$DOC01" 2>/dev/null | sed -E 's/^\| [A-Z_]+ \| 09 §2：([A-Z_]+).*/\1/' | sort -u || true)
B09=$(sed -n '/^### .*AgentErrorCode=/,/^$/p' "$DOC09" 2>/dev/null | grep -oE 'AgentErrorCode=[A-Z_]+' | sed 's/AgentErrorCode=//' | sort -u || true)
[ -z "$B09" ] && B09=$(grep -oE 'AgentErrorCode=[A-Z_]+' "$DOC09" 2>/dev/null | sed 's/AgentErrorCode=//' | sort -u || true)
if [ -z "$A01" ]; then
  echo -e "  ${RED}❌${NC} 01 §17.9: 单向指针列未提取到（R-1395: 每行必须引用 09 §2 条目号）"
  FAILED=$((FAILED+1))
else
  missing_in_09=$(comm -23 <(echo "$A01") <(echo "$B09") || true)
  if [ -n "$missing_in_09" ]; then
    echo -e "  ${RED}❌${NC} 01 §17.9 引用, 09 §2 条目无: $(echo $missing_in_09 | tr '\n' ' ')"
    FAILED=$((FAILED+1))
  fi
fi

echo ""
echo "──────────────────────────────"
echo "  失败合计: $FAILED"
echo ""
[ "$FAILED" -eq 0 ] && echo -e "${GREEN}✅ 错误码单一数据源验证通过——yaml/09 §2.5/07 内联枚举/01 单向指针一致${NC}" && exit 0
echo -e "${RED}❌ $FAILED 项错误码漂移。R-1300: error-codes-source.yaml 为唯一维护侧, 变更顺序=yaml→09 §2.5→07; R-1395: 01 §17.9 单向指针⊆09 §2。${NC}"
exit 1
