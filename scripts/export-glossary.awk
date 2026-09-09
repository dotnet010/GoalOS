# export-glossary.awk——00统一术语表.md 表格→glossary.yaml 提取器（单进程）。
# 背景：原 bash 版每行 7 个外部进程（echo|grep|sed|cut）——Windows Git Bash
# (MSYS) fork 开销 ≈0.5s/行×640 行=15 分钟级不可用（2026-09-10 实机超时实锤）。
# 语义=原 bash 逐行等值（对照验证：110 terms 全量一致）：
#   表头命中（含"术语"+"说明"）→ in_table；分隔行/空行/##/--- 退出；
#   术语行（| **Term** | desc |）→ 列提取（cols[1]=行首"|"前空段，cols[2]=term，
#   cols[3..]=desc——等价 bash 的 sed 's/^| *//'|cut 语义）；
#   豁免（正确术语/术语.*值）；desc 含 struct|interface|字段|Schema|schema|YAML|JSON
#   → has_schema=1；单引号转义 ''。
{
  if (in_table == 0 && $0 ~ /^\|.*术语.*\|.*说明/) { in_table=1; next }
  if (in_table == 1 && $0 ~ /^\|[-: |]+\|/) next
  if (in_table == 1 && ($0 ~ /^$/ || $0 ~ /^## / || $0 ~ /^---$/)) { in_table=0; next }
  if (in_table == 1 && $0 ~ /^\|.*\*\*.*\*\*.*\|/) {
    n=split($0, cols, "|")
    term=cols[2]
    sub(/^ */,"",term); sub(/ *$/,"",term)
    gsub(/\*\*/,"",term)
    if (term ~ /正确术语/ || term ~ /术语.*值/) next
    if (term == "") next
    desc=""
    for (i=3;i<=n;i++) { if (i>3) desc=desc "|"; desc=desc cols[i] }
    sub(/^ */,"",desc); sub(/ *$/,"",desc)
    sub(/ *\| *$/,"",desc)
    has=0
    if (desc ~ /struct|interface|字段|Schema|schema|YAML|JSON/) has=1
    gsub(/'/,"''",desc)
    print "  - name: '" term "'"
    print "    description: '" desc "'"
    print "    has_schema: " has
    print "    defined_in: '05软件架构文档.md'"
  }
}
