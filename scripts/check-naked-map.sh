#!/bin/bash
# check-naked-map.sh — CI 反欺骗层：AST 扫描裸 map[K]V 包级声明
# 依据: R-735 CI 三层强制 — 层 1
# 规则: 包级可变 map[K]V 声明 → CI 失败。只读查表（字面量初始化）→ WARN。
# CI2 fix: 区分可变共享状态 vs 只读查找表。
# 排除: _test.go 文件、safemap 包自身、注释中的 map 声明

set -euo pipefail

ROOT_DIR="${1:-.}"
FAILED=0

echo "=== check-naked-map: 扫描裸 map[K]V 包级声明 ==="

while IFS= read -r file; do
    VIOLATIONS=$(grep -nE '^\s*(var\s+\w+\s+map\[|var\s+\w+\s*=\s*make\(map\[|var\s+\w+\s*=\s*map\[)' "$file" 2>/dev/null || true)

    if [ -n "$VIOLATIONS" ]; then
        # CI2: 区分只读查表（字面量初始化含 {）vs 可变共享状态
        MUTABLE=$(echo "$VIOLATIONS" | grep -vE 'map\[.*\].*\{' || true)
        READONLY=$(echo "$VIOLATIONS" | grep -E 'map\[.*\].*\{' || true)
        if [ -n "$READONLY" ]; then
            echo ""
            echo "  WARN: $file - 只读查表 map:"
            echo "$READONLY" | while read -r line; do echo "   $line"; done
        fi
        if [ -n "$MUTABLE" ]; then
            echo ""
            echo "  FAIL: $file - 可变裸 map 包级声明:"
            echo "$MUTABLE" | while read -r line; do echo "   $line"; done
            echo "  修复: 使用 safemap.New[K,V]()"
            FAILED=1
        fi
    fi
done < <(find "$ROOT_DIR" -name "*.go" \
    ! -name "*_test.go" \
    ! -path "*/safemap/*" \
    ! -path "*/vendor/*" \
    ! -path "*/.git/*" 2>/dev/null)

# CI2: 测试文件中的裸 map 降级为 WARN
while IFS= read -r file; do
    TEST_VIOLATIONS=$(grep -nE '^\s*(var\s+\w+\s+map\[|var\s+\w+\s*=\s*make\(map\[|var\s+\w+\s*=\s*map\[)' "$file" 2>/dev/null || true)
    if [ -n "$TEST_VIOLATIONS" ]; then
        echo ""
        echo "  WARN: $file - 测试文件发现裸 map 声明（建议修复）:"
        echo "$TEST_VIOLATIONS" | while read -r line; do echo "   $line"; done
        echo "  建议: 测试文件中的共享状态也应使用 safemap.New[K,V]()"
    fi
done < <(find "$ROOT_DIR" -name "*_test.go" \
    ! -path "*/vendor/*" \
    ! -path "*/.git/*" 2>/dev/null)

echo ""
if [ "$FAILED" -eq 0 ]; then
    echo "=== check-naked-map: PASSED ==="
    exit 0
else
    echo "=== check-naked-map: FAILED ==="
    exit 1
fi
