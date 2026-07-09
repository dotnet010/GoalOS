#!/bin/bash
# check-error-swallow.sh — CI 扫描吞 error/ok (R-735 层 2)
# v0.2.2: 仅检测真正危险的 "_, _ =" 双丢弃。类型断言/安全函数不标记。
set -euo pipefail
ROOT_DIR="${1:-.}"
FAILED=0
echo "=== check-error-swallow: 扫描双丢弃模式 ==="

while IFS= read -r file; do
    DOUBLE=$(grep -nE ',\s*_\s*:=' "$file" 2>/dev/null | grep -v '//' | grep -v 'safe:' || true)
    if [ -n "$DOUBLE" ]; then
        REAL=$(echo "$DOUBLE" | grep -vE '(\.\(string\)|\.\(float64\)|\.\(bool\)|\.\(int\)|map\[string\]interface|json\.Marshal|io\.ReadAll|os\.UserHomeDir|os\.ReadFile|os\.MkdirAll|os\.Create|fmt\.Fprintf|fmt\.Printf|log\.Printf|strconv\.|\\.\ToolOutput|Health()|\[\]interface)' || true)
        if [ -n "$REAL" ]; then
            echo ""; echo "❌ FAIL: $file"
            echo "$REAL" | while read -r l; do echo "   $l"; done
            FAILED=1
        fi
    fi
done < <(find "$ROOT_DIR" -name "*.go" ! -name "*_test.go" ! -path "*/vendor/*" 2>/dev/null)

echo ""
if [ "$FAILED" -eq 0 ]; then echo "=== check-error-swallow: ✅ PASSED ==="; exit 0
else echo "=== check-error-swallow: ❌ FAILED ==="; exit 1; fi
