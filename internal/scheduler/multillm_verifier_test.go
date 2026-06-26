package scheduler_test

import (
	"os"
	"testing"

	"github.com/goalos/goalos/internal/missionengine"
	"github.com/goalos/goalos/internal/scheduler"
)

// TestMultiLLMVerifier_DualProvider 验证双 Provider 并行审查。
// 需要设置环境变量 GOALOS_LLM_API_KEY 和 OPENROUTER_API_KEY。
// CI 中使用 -short 跳过。
func TestMultiLLMVerifier_DualProvider(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping multi-LLM integration test in short mode")
	}
	glmKey := os.Getenv("GOALOS_LLM_API_KEY")
	orKey := os.Getenv("OPENROUTER_API_KEY")
	if glmKey == "" || orKey == "" {
		t.Skip("skipping: GOALOS_LLM_API_KEY or OPENROUTER_API_KEY not set")
	}

	providers := []scheduler.ProviderClient{
		{
			Name:  "glm",
			Model: "glm-5.1",
			Client: missionengine.NewCloudLLMClient(
				"https://ws-hwiv1ueutcxpjuzq.cn-beijing.maas.aliyuncs.com/compatible-mode/v1",
				glmKey,
				"glm-5.1",
			),
		},
		{
			Name:  "nemotron",
			Model: "nvidia/nemotron-3-ultra-550b-a55b:free",
			Client: missionengine.NewCloudLLMClient(
				"https://openrouter.ai/api/v1",
				orKey,
				"nvidia/nemotron-3-ultra-550b-a55b:free",
			),
		},
	}

	verifier := scheduler.NewMultiLLMVerifier(providers)

	code := `package main
func add(a, b int) int { return a + b }
func main() { println(add(1, 2)) }`

	t.Log("Running Multi-LLM verification with 2 providers in parallel...")
	verdict, err := verifier.Verify(code, "test-action-001")
	if err != nil {
		t.Fatalf("MultiLLM verification failed: %v", err)
	}

	t.Logf("Verdict: %s (score=%.2f, consensus=%v, needs_meta=%v)",
		verdict.Result, verdict.WeightedScore, verdict.Consensus, verdict.NeedsMeta)
	for _, v := range verdict.Votes {
		t.Logf("  %s/%s: %s — %s", v.Provider, v.Model, v.Vote, v.Reasoning[:min(80, len(v.Reasoning))])
	}

	if len(verdict.Votes) == 0 {
		t.Error("No valid votes received from any provider")
	}
}

func min(a, b int) int { if a < b { return a }; return b }
