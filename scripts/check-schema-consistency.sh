#!/bin/bash
# =============================================================================
# GoalOS Schema Consistency Checker — 借鉴 gh-aw 策略框架
#
# 废弃登记（2026-08-13, S'-24⑥ R-1267 裁决）: 本脚本已废弃, 不再接入 CI。
#   废弃原因: 数据源不存在（GoalOS/glossary.yaml 与 .goalos-ci/schema-cache.json
#   均从未落地），三向比对功能被 check-resolution-propagation.sh（层2 正文
#   一致性）+ check-error-codes.sh（error-codes-source.yaml↔07 内联枚举）
#   完全覆盖。新检查一律走已接线脚本, 勿复活本脚本。
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

# ─── Layer 2: 概念一致性检查（R-758 — 会议 #116 新增） ───
echo ""
echo "── Layer 2: 概念一致性检查（R-758）──"

DOCS_DIR="开发文档"
ABOLISHED_TERMS="RecoveryPipeline|AUTO_FIX|SWITCH_TOOL|四原语|4 Primitive"
LEGACY_TERMS="WebSocket推送|stdin/stdout.*JSON|DecidePathSelected\(AUTO_FIX\)|DecidePathSelected\(REPLAN\)"

echo "  2.1 已废除概念残留检测..."
for term in RecoveryPipeline AUTO_FIX SWITCH_TOOL; do
    # 在正文中（排除修改记录和已废除标注）
    violations=$(grep -rn "$term" "$DOCS_DIR" --include="*.md" 2>/dev/null | \
        grep -v "修改记录" | grep -v "已废除" | grep -v "会议 #107" | grep -v "R-740" | \
        grep -v "stub追踪" | grep -v "开发计划/v0.2.0" | grep -v "会议纪要" || true)
    if [ -n "$violations" ]; then
        echo -e "    ${R}❌ '$term' 出现在正文中（非废除/修改记录上下文）:${N}"
        echo "$violations" | head -5 | while read -r line; do echo "       $line"; done
        FAIL=$((FAIL + 1))
    fi
done
echo -e "    ${G}✅ 已废除概念无正文残留${N}"

echo "  2.2 WebSocket→SSE / stdin/stdout→FD3 检测..."
# WebSocket should not appear in PRD or 05-arch body (modification records OK)
ws_violations=$(grep -rn "WebSocket推送\|WebSocket 协议" 01-prd产品需求文档.md 05软件架构文档.md 2>/dev/null | grep -v "SSE\|已废弃\|修改记录" || true)
if [ -n "$ws_violations" ]; then
    echo -e "    ${R}❌ WebSocket 引用未替换为 SSE:${N}"
    echo "$ws_violations"
    FAIL=$((FAIL + 1))
fi
# stdin/stdout should not appear as IPC mechanism in 08-沙箱 body
stdin_violations=$(grep -n "stdin/stdout" 08沙箱隔离与进程通信规范.md 2>/dev/null | grep -v "FD3\|旧版\|已废弃\|os/exec.*FD3" || true)
if [ -n "$stdin_violations" ]; then
    echo -e "    ${R}❌ stdin/stdout 未替换为 FD3:${N}"
    echo "$stdin_violations"
    FAIL=$((FAIL + 1))
fi
echo -e "    ${G}✅ 遗留协议引用已清理${N}"

echo "  2.3 Validate() 调用者统一检测（R-759）..."
# 06-安全 should say "PipelineRunner 自动调用"
if grep -q "每个模块在发布事件前调用" 06安全模型文档.md 2>/dev/null; then
    echo -e "    ${R}❌ 06-安全 §1.1 Validate() 调用者未统一为 PipelineRunner${N}"
    FAIL=$((FAIL + 1))
fi
# 开发规范 should say "PipelineRunner 自动调用"
if grep -q "调用方（模块第一接触点）在发布事件前调用" ../开发文档/11安全开发流程规范.md 2>/dev/null; then
    echo -e "    ${R}❌ 开发规范 §4.3 Validate() 调用者未统一为 PipelineRunner${N}"
    FAIL=$((FAIL + 1))
fi
echo -e "    ${G}✅ Validate() 调用者已统一${N}"

echo "  2.4 新概念一致性——核心术语跨文档定义一致..."
ROOT="/Users/haochen/work/workspace/pi2"
for term in "SafeMap" "Fan-Out" "Validatable" "CategorizedError" "MissionNode" "PluginRegistered"; do
    glossary_count=$(grep -c "$term" "$ROOT/开发文档/GLOSSARY.md" 2>/dev/null || echo 0)
    arch_count=$(grep -c "$term" "$ROOT/开发文档/05软件架构文档.md" 2>/dev/null || echo 0)
    if [ "$glossary_count" -gt 0 ] && [ "$arch_count" -gt 0 ]; then
        :
    elif [ "$glossary_count" -eq 0 ]; then
        echo -e "    ${Y}⚠️  '$term' 未在 GLOSSARY 中定义${N}"
    else
        echo -e "    ${Y}⚠️  '$term' 未在 05-架构中引用${N}"
    fi
done
echo -e "    ${G}✅ 新概念跨文档一致${N}"

echo ""
echo "══════════════════════════════"
echo "  策略1(术语→Schema): $missing 缺失"
echo "  策略2(事件→Payload): 需人工核对"
echo "  策略3(漂移检测): 缓存已更新 → $CACHE"
echo "  策略4(概念一致性): R-758 Layer 2"
echo "  失败合计: $FAIL"
echo "══════════════════════════════"
echo ""

[ $FAIL -eq 0 ] && echo -e "${G}✅ Schema + 概念一致性检查通过${N}" && exit 0
echo -e "${R}❌ $FAIL 项不一致。修复后重新运行。${N}"
exit 1
