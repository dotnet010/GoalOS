#!/bin/bash
# 从 00统一术语表.md 表格提取术语→生成 glossary.yaml（机器可读）
# 用法: bash scripts/export-glossary-yaml.sh > GoalOS/glossary.yaml
# 2026-09-10 性能修复：原实现每行 7 个外部进程（echo|grep|sed|cut）——Windows
# Git Bash (MSYS) fork 开销 ≈0.5s/行×640 行=15 分钟级不可用（实机超时实锤）；
# 换 awk 单进程等值实现（scripts/export-glossary.awk——语义逐字保持，110 terms
# 全量对照一致）。产物区别=修复了旧版 desc 尾随空格漂移（sed 链残余尾空格）。
set -euo pipefail

GLOSSARY_MD="${1:-开发文档/00统一术语表.md}"
OUTPUT="${2:-/dev/stdout}"

{
echo "# GoalOS 00统一术语表 — 机器可读 YAML（从 $GLOSSARY_MD 自动生成）"
echo "# 生成时间: $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
echo "# 真相来源: $GLOSSARY_MD — 本文件是其派生文件。不一致时以 Markdown 为准。"
echo ""
echo "terms:"
awk -f "$(dirname "$0")/export-glossary.awk" "$GLOSSARY_MD"
} > "$OUTPUT"

if [ "$OUTPUT" != "/dev/stdout" ]; then
  echo "Exported $(grep -c 'name:' "$OUTPUT") terms to $OUTPUT" >&2
fi
