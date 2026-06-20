#!/bin/bash
# GoalOS v1.0.1 多平台 Release 上传脚本
# 用法: GITHUB_TOKEN="ghp_xxx" ./upload-release.sh

set -e
TOKEN="${GITHUB_TOKEN:?请设置 GITHUB_TOKEN 环境变量}"
REPO="dotnet010/GoalOS"
BINARIES_DIR="/tmp/goalos-release"

echo "=== 创建 Release v1.0.1 ==="
RELEASE_ID=$(curl -s -X POST "https://api.github.com/repos/$REPO/releases" \
  -H "Authorization: token $TOKEN" \
  -H "Accept: application/vnd.github+json" \
  -d '{"tag_name":"v1.0.1","name":"GoalOS v1.0.1 — 多平台 Release","body":"## 多平台构建\n\n| 平台 | 文件 |\n|------|------|\n| macOS Apple Silicon | goalos-darwin-arm64.tar.gz |\n| macOS Intel | goalos-darwin-amd64.tar.gz |\n| Linux x86_64 | goalos-linux-amd64.tar.gz |\n| Linux ARM64 | goalos-linux-arm64.tar.gz |\n| Windows x64 | goalos-windows-amd64.zip |\n\n### 安装\n\n```bash\n# macOS/Linux\ntar -xzf goalos-*.tar.gz\nchmod +x goalos-daemon goalos\nmv goalos-daemon goalos /usr/local/bin/\n```","draft":false,"prerelease":false}' | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")

echo "Release ID: $RELEASE_ID"

echo ""
echo "=== 上传二进制文件 ==="
for f in "$BINARIES_DIR"/*; do
  name=$(basename "$f")
  echo -n "Uploading $name... "
  HTTP=$(curl -s -o /dev/null -w "%{http_code}" \
    -H "Authorization: token $TOKEN" \
    -H "Content-Type: application/octet-stream" \
    --data-binary "@$f" \
    "https://uploads.github.com/repos/$REPO/releases/$RELEASE_ID/assets?name=$name")
  echo "$HTTP"
done

echo ""
echo "✅ v1.0.1 Release 发布完成"
echo "查看: https://github.com/$REPO/releases/tag/v1.0.1"
