#!/bin/bash
# 从 GLOSSARY.md 表格提取术语→生成 glossary.yaml（机器可读）
# 用法: bash scripts/export-glossary-yaml.sh > GoalOS/glossary.yaml
set -euo pipefail

GLOSSARY="${1:-开发文档/GLOSSARY.md}"
OUTPUT="${2:-/dev/stdout}"

{
echo "# GoalOS GLOSSARY — 机器可读 YAML（从 $GLOSSARY 自动生成）"
echo "# 生成时间: $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
echo "# 真相来源: $GLOSSARY — 本文件是其派生文件。不一致时以 Markdown 为准。"
echo ""
echo "terms:"

in_table=0
while IFS= read -r line; do
    # 检测表格开始（"| 术语" 或 "| 正确术语" 行）
    if echo "$line" | grep -qE '^\|.*术语.*\|.*说明'; then
        in_table=1
        continue
    fi

    # 表格分隔行 |---| 或 |:---| 跳过
    if [ "$in_table" -eq 1 ] && echo "$line" | grep -qE '^\|[-: |]+\|'; then
        continue
    fi

    # 空行或 ## 标题 → 退出表格
    if [ "$in_table" -eq 1 ] && echo "$line" | grep -qE '^$|^## |^---$'; then
        in_table=0
        continue
    fi

    # 提取术语行: | **Term** | Description |
    if [ "$in_table" -eq 1 ] && echo "$line" | grep -qE '^\|.*\*\*.*\*\*.*\|'; then
        # 提取术语名
        term=$(echo "$line" | sed 's/^| *//' | cut -d'|' -f1 | sed 's/\*\*//g' | sed 's/^ *//;s/ *$//')
        # 提取说明
        desc=$(echo "$line" | sed 's/^| *//' | cut -d'|' -f2- | sed 's/^ *//;s/ *$//;s/|$//')
        # 清理：移除末尾残留的 |
        desc=$(echo "$desc" | sed 's/ *| *$//')

        [ -z "$term" ] && continue
        # 跳过非术语行（如"正确术语 | 废弃术语 | 说明"这种三列表头）
        echo "$term" | grep -qE '正确术语|术语.*值' && continue

        # 判断是否有 Schema（描述中包含关键词）
        has_schema=0
        echo "$desc" | grep -qE 'struct|interface|字段|Schema|schema|YAML|JSON' && has_schema=1

        # YAML 安全输出——用单引号括描述，内部单引号转义为 ''
        desc_safe=$(echo "$desc" | sed "s/'/''/g")

        echo "  - name: '$term'"
        echo "    description: '$desc_safe'"
        echo "    has_schema: $has_schema"
        echo "    defined_in: '05软件架构文档.md'"
    fi
done < "$GLOSSARY"
} > "$OUTPUT"

[ "$OUTPUT" != "/dev/stdout" ] && echo "Exported $(grep -c 'name:' "$OUTPUT") terms to $OUTPUT" >&2
