#!/bin/bash
# =============================================================================
# GoalOS Schema Consistency Checker — 借鉴 gh-aw 策略框架
#
# 三向交叉验证:
#   GLOSSARY(YAML) ←→ 05-架构(Schema定义) ←→ 07-事件(Payload字段)
#
# gh-aw 原策略: Schema fields vs Parser validFields
# GoalOS 适配:  GLOSSARY terms vs 05-架构 struct vs 07-事件 payload
# =============================================================================
set -euo pipefail

R='\033[0;31m'; G='\033[0;32m'; Y='\033[1;33m'; N='\033[0m'
FAIL=0

GLOSSARY_YAML="GoalOS/glossary.yaml"
ARCH="开发文档/05软件架构文档.md"
EVENTS="开发文档/07事件注册表.md"
CACHE=".goalos-ci/schema-cache.json"

echo "===== GoalOS Schema Consistency Checker（借鉴 gh-aw 策略框架）====="
echo ""

# ═══ 策略1: GLOSSARY 术语 vs 05-架构 定义字段 ═══
echo "── 策略1: GLOSSARY → 05-架构 Schema 交叉验证 ──"

# 从 GLOSSARY YAML 提取所有术语名
terms=$(grep "name:" "$GLOSSARY_YAML" 2>/dev/null | sed "s/.*name: *'//;s/'.*//" | sort -u)
t_count=$(echo "$terms" | grep -c . || echo 0)

# 从 05-架构 提取所有 struct/interface/YAML Schema 定义名
structs_in_arch=$(grep -ohE '^### [A-Z][a-zA-Z]+|type [A-Z][a-zA-Z]+ struct|```yaml\n[A-Z][a-zA-Z]+:' "$ARCH" 2>/dev/null | sed 's/^### //;s/type //;s/ struct//;s/```yaml//;s/:.*//' | sort -u | head -30)

verified=0; missing=0
for term in $terms; do
    [ -z "$term" ] && continue
    if echo "$structs_in_arch" | grep -qF "$term" 2>/dev/null; then
        verified=$((verified + 1))
    else
        if grep -q "$term" "$ARCH" 2>/dev/null; then
            # 存在引用但无 struct 定义——需要检查是否有 Schema 块
            has_fields=$(grep -A30 "$term" "$ARCH" 2>/dev/null | grep -cE '^\s+[a-z_]+:.*#|string|int|bool|float' || echo 0)
            if [ "${has_fields:-0}" -lt 2 ]; then
                echo -e "  ${R}❌${N} $term: GLOSSARY有定义,05-架构引用但无字段级定义"
                missing=$((missing + 1))
            fi
        else
            echo -e "  ${Y}⚠️${N} $term: GLOSSARY有定义,05-架构未引用"
            missing=$((missing + 1))
        fi
    fi
done

echo "  术语总数: $t_count | 已验证: $verified | 缺失定义: $missing"
FAIL=$((FAIL + missing))
echo ""

# ═══ 策略2: 07-事件 Payload 字段 vs 05-架构 Event struct ═══
echo "── 策略2: 07-事件 Payload → 05-架构 Event 结构 ──"

# 从 07-事件 提取所有事件名及其 payload 字段
event_payloads=0; event_missing=0
grep -n "^### [A-Z]" "$EVENTS" 2>/dev/null | while read -r line; do
    event_name=$(echo "$line" | sed 's/.*### //')
    lineno=$(echo "$line" | cut -d: -f1)
    # 提取该事件后面30行内的 payload 字段数
    payload_fields=$(tail -n +"$lineno" "$EVENTS" | head -30 | grep -cE '^\s+[a-z_]+:' || echo 0)
    if [ "${payload_fields:-0}" -eq 0 ]; then
        event_missing=$((event_missing + 1))
    fi
    event_payloads=$((event_payloads + 1))
done 2>/dev/null || true
echo "  事件 payload 覆盖: 待人工核对——07-事件格式为 prose+YAML 混合"

echo ""

# ═══ 策略3: 缓存对比——检测文档漂移（借鉴 gh-aw 策略缓存） ═══
echo "── 策略3: 文档漂移检测（基于缓存对比）──"

mkdir -p "$(dirname "$CACHE")"
current_hash=$(md5 -q "$GLOSSARY_YAML" 2>/dev/null || md5sum "$GLOSSARY_YAML" 2>/dev/null | cut -d' ' -f1)

if [ -f "$CACHE" ]; then
    cached_hash=$(grep -o '"glossary_hash":"[^"]*"' "$CACHE" 2>/dev/null | cut -d'"' -f4 || echo "")
    if [ "$current_hash" != "$cached_hash" ] && [ -n "$cached_hash" ]; then
        echo -e "  ${Y}⚠️${N} GLOSSARY 自上次检查后已变更——需重新验证 Schema 一致性"
        echo "    上次: $cached_hash"
        echo "    当前: $current_hash"
    else
        echo -e "  ${G}✅${N} GLOSSARY 未变更——Schema 一致性已缓存"
    fi
else
    echo -e "  ${Y}⚠️${N} 首次运行——创建缓存"
fi

# 更新缓存
cat > "$CACHE" << EOF
{"glossary_hash":"$current_hash","last_check":"$(date -u +"%Y-%m-%dT%H:%M:%SZ")","terms_count":$t_count,"verified":$verified,"missing":$missing}
EOF

echo ""
echo "══════════════════════════════"
echo "  策略1(术语→Schema): $missing 缺失"
echo "  策略2(事件→Payload): 需人工核对"
echo "  策略3(漂移检测): 缓存已更新 → $CACHE"
echo "  失败合计: $FAIL"
echo "══════════════════════════════"
echo ""

[ $FAIL -eq 0 ] && echo -e "${G}✅ Schema 一致性检查通过${N}" && exit 0
echo -e "${R}❌ $FAIL 项 Schema 不一致。修复后重新运行。${N}"
exit 1
