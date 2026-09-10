package config

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestCanonicalVersion_Shape（任务 8.5——版本常量防僵尸闸）：
// (1) 形态=semver X.Y.Z 正式版号（无预发布/构建后缀——发布纪律）；
// (2) 声明锚点形态=check-version-sync.sh 的 grep 锚点逐字一致（脚本闸与本闸双锚同步——
// 锚点漂移=闸失效无人知晓，本断言机检钉死）；
// (3) 常量值与源码声明值相等（编译期常量未经 ldflags 覆写——R-361 唯一来源语义）。
// 背景：常量曾滞留 0.1.3 三个版本无人发现——机制化防复发（脚本闸=tag CI；本闸=形态+锚点）。
func TestCanonicalVersion_Shape(t *testing.T) {
	if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(CanonicalVersion) {
		t.Fatalf("CanonicalVersion %q 形态失真（须为 X.Y.Z 正式版号）", CanonicalVersion)
	}
	if strings.ContainsAny(CanonicalVersion, "-+") {
		t.Fatalf("CanonicalVersion %q 含预发布/构建后缀——只承载正式版号", CanonicalVersion)
	}
	src, err := os.ReadFile("config.go")
	if err != nil {
		t.Fatalf("读 config.go 失败: %v", err)
	}
	anchor := `const CanonicalVersion = "` + CanonicalVersion + `"`
	if !strings.Contains(string(src), anchor) {
		t.Fatalf("声明锚点漂移：config.go 中找不到 %q（check-version-sync.sh grep 锚点同形态）", anchor)
	}
}
