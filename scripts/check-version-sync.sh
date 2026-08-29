#!/usr/bin/env bash
# check-version-sync.sh——CanonicalVersion（R-361 权威常量）与 git tag 一致性闸。
# 背景：会议 #237 Linus 裁定「发版前版本号不得落后」；事故史=常量曾滞留 0.1.3
# 三个版本无人发现（任务 8.5 实证——声明了"CI 检查"措辞但机制从未存在，僵尸声明）。
#
# 行为：
#   tag 上下文（GITHUB_REF_NAME=vX.Y.Z，docker-publish.yml）→ 硬闸：不等即红。
#   无 tag 上下文（本地/分支 CI）→ 形态检查（semver X.Y.Z）+ SKIP 明示（不假装做了校验）。
set -euo pipefail

CONST_FILE="internal/config/config.go"
CONST=$(grep -E 'const CanonicalVersion = "[0-9]+\.[0-9]+\.[0-9]+"' "$CONST_FILE" \
  | sed -E 's/.*"([0-9]+\.[0-9]+\.[0-9]+)".*/\1/')

if [ -z "$CONST" ]; then
  echo "❌ check-version-sync: $CONST_FILE 中找不到合规 CanonicalVersion 常量（形态=const CanonicalVersion = \"X.Y.Z\"）"
  exit 1
fi

TAG="${GITHUB_REF_NAME:-}"
if [[ ! "$TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "SKIP check-version-sync: 无 tag 上下文（GITHUB_REF_NAME=${TAG:-空}）；常量形态校验通过（$CONST）"
  exit 0
fi

TAGVER="${TAG#v}"
if [ "$CONST" != "$TAGVER" ]; then
  echo "❌ check-version-sync: tag $TAG 与 CanonicalVersion $CONST 不一致——发版前版本号不得落后（会议 #237）"
  exit 1
fi
echo "✅ check-version-sync: tag $TAG == CanonicalVersion $CONST"
