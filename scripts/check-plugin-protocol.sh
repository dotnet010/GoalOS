#!/bin/bash
# =============================================================================
# GoalOS Plugin 内部协议合规检查 — R-725（会议 #106 深度复盘）
#
# 验证所有 Plugin 二进制遵守 GoalOS 内部 IPC 协议（v2.0 两行 HMAC）。
# 协议边界: GoalOS daemon ↔ Plugin 子进程。不扩散到外部。
#
# 退出码: 0=全部合规, 1=存在违规 Plugin
# =============================================================================
set -euo pipefail

PLUGIN_DIR="${HOME}/.goalos/plugins"
EXPECTED_PROTOCOL="v2.0-two-line-hmac"
FAIL=0

echo "===== GoalOS Plugin 内部协议合规检查 ====="
echo "  期望协议版本: ${EXPECTED_PROTOCOL}"
echo ""

for plugin_json in "$PLUGIN_DIR"/*/*/plugin.json; do
    [ -f "$plugin_json" ] || continue
    plugin_dir=$(dirname "$plugin_json")
    plugin_name=$(python3 -c "import json; print(json.load(open('$plugin_json'))['name'])" 2>/dev/null || echo "unknown")
    plugin_binary=$(python3 -c "import json; print(json.load(open('$plugin_json'))['binary'])" 2>/dev/null || echo "")

    echo "── Plugin: $plugin_name ($plugin_dir) ──"

    # 检查 1: 二进制是否存在
    binary_path="$plugin_dir/$plugin_binary"
    if [ ! -f "$binary_path" ]; then
        echo "  ❌ 二进制不存在: $binary_path"
        FAIL=$((FAIL + 1))
        continue
    fi
    echo "  ✅ 二进制存在"

    # 检查 2: 二进制是否输出两行 HMAC 协议（向 stdin 发送 init+execute，检查 stdout 格式）
    test_output=$(echo '{"type":"init","protocol_version":"v2.0-two-line-hmac","session_token":"test123","workspace":"/tmp","tmp":"/tmp","capabilities":["shell.execute"]}
{"type":"execute","action_id":"test","action_type":"shell.execute","target":"echo ok","params":{},"timeout_ms":3000}
{"type":"shutdown","reason":"completed"}' | "$binary_path" 2>/dev/null | head -2)

    line1=$(echo "$test_output" | head -1)
    line2=$(echo "$test_output" | head -2 | tail -1)

    # v2.0 协议: 第一行=64字符 HMAC hex, 第二行=合法 JSON
    if echo "$line1" | grep -qE '^[0-9a-fA-F]{64}$'; then
        echo "  ✅ 第一行: HMAC-SHA256 hex (64字符)"
    else
        echo "  ❌ 第一行: 不是 64 字符 hex——Plugin 未使用 v2.0 两行 HMAC 协议"
        echo "     实际: ${line1:0:80}..."
        FAIL=$((FAIL + 1))
        continue
    fi

    if echo "$line2" | python3 -c "import sys,json; json.loads(sys.stdin.read())" 2>/dev/null; then
        echo "  ✅ 第二行: 合法 JSON payload"
    else
        echo "  ❌ 第二行: 不是合法 JSON"
        FAIL=$((FAIL + 1))
        continue
    fi

    # 检查 3: JSON payload 中不含 "hmac" 字段（旧协议残留）
    if echo "$line2" | python3 -c "import sys,json; d=json.loads(sys.stdin.read()); assert 'hmac' not in d, '旧协议残留'" 2>/dev/null; then
        echo "  ✅ 无旧协议残留（hmac 字段）"
    else
        echo "  ❌ JSON 中含 'hmac' 字段——旧协议残留"
        FAIL=$((FAIL + 1))
    fi
done

echo ""
echo "══════════════════════════════"
echo "  失败: $FAIL"
echo "══════════════════════════════"

[ $FAIL -eq 0 ] && echo "✅ 所有 Plugin 内部协议合规" && exit 0
echo "❌ $FAIL 个 Plugin 违反内部协议——修复后重新运行" && exit 1
