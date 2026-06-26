package releasecheck_test

import (
	"testing"

	"github.com/goalos/goalos/internal/releasecheck"
)

// TestReleaseReadiness_All 发布前完整检查。CI 中强制执行。
func TestReleaseReadiness_All(t *testing.T) {
	repoRoot := "../.." // GoalOS/
	pluginsDir := repoRoot + "/plugins/capability"

	results, err := releasecheck.RunAll(repoRoot, pluginsDir)
	if err != nil {
		for _, r := range results {
			status := "PASS"
			if !r.Passed { status = "FAIL" }
			t.Logf("  [%s] %s: %s", status, r.Name, r.Detail)
		}
		t.Fatalf("发布就绪检查失败: %v", err)
	}

	t.Log("全部 5 项发布就绪检查通过 ✅")
	for _, r := range results {
		t.Logf("  ✅ %s: %s", r.Name, r.Detail)
	}
}
