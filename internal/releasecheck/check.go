// Package releasecheck — L6 发布就绪测试（v0.1.1 复盘新增）。
// 发布前强制执行。任一项不通过 = 禁止发布。
//
// 设计依据：R-010, R-011, R-012, R-013。
package releasecheck

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CheckResult 是单次检查的结果。
type CheckResult struct {
	Name    string
	Passed  bool
	Detail  string
}

// RunAll 执行全部 5 项发布就绪检查。任一失败返回 error。
func RunAll(repoRoot, pluginsDir string) ([]CheckResult, error) {
	var results []CheckResult

	results = append(results, checkSecrets(repoRoot))
	results = append(results, checkTestConfigNotInGit(repoRoot))
	results = append(results, checkPluginSignatures(pluginsDir))
	results = append(results, checkConfigSchema(repoRoot))
	results = append(results, checkArchitectureContracts(repoRoot))

	for _, r := range results {
		if !r.Passed {
			return results, fmt.Errorf("release check FAILED: %s — %s", r.Name, r.Detail)
		}
	}
	return results, nil
}

// checkSecrets 敏感信息扫描。
func checkSecrets(repoRoot string) CheckResult {
	patterns := []string{
		`ghp_[A-Za-z0-9]{36}`,       // GitHub PAT
		`sk-ant-[A-Za-z0-9_-]{32,}`, // Anthropic
		`sk-or-[A-Za-z0-9_-]{32,}`,  // OpenRouter
		`sk-ws-[A-Za-z0-9_.-]{32,}`, // Alibaba Bailian
		`x-api-key\s*[:=]\s*["\x27]?[A-Za-z0-9_-]{20,}`, // generic API key header
	}

	for _, pattern := range patterns {
		cmd := exec.Command("grep", "-rn", pattern, repoRoot,
			"--include=*.go", "--include=*.yaml", "--include=*.yml", "--include=*.json",
		)
		cmd.Env = append(os.Environ(), "HOME="+os.Getenv("HOME"))
		out, _ := cmd.Output()
		lines := strings.TrimSpace(string(out))
		if lines != "" {
			// 排除已知安全文件
			if !strings.Contains(lines, ".git/") && !strings.Contains(lines, ".env") {
				return CheckResult{Name: "secrets-scan", Passed: false,
					Detail: "检测到可能的密钥:\n" + lines}
			}
		}
	}
	return CheckResult{Name: "secrets-scan", Passed: true, Detail: "无密钥泄漏"}
}

// checkTestConfigNotInGit 测试配置文件不应被 git 追踪。
func checkTestConfigNotInGit(repoRoot string) CheckResult {
	cmd := exec.Command("git", "ls-files")
	cmd.Dir = repoRoot
	out, _ := cmd.Output()
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "daemon.test.yaml") {
			return CheckResult{Name: "test-config-git", Passed: false,
				Detail: "daemon.test.yaml 被 git 追踪——应立即从追踪中移除"}
		}
	}
	return CheckResult{Name: "test-config-git", Passed: true, Detail: "测试配置文件已保护"}
}

// checkPluginSignatures 插件二进制签名与 plugin.json 一致。
func checkPluginSignatures(pluginsDir string) CheckResult {
	var failures []string
	filepath.Walk(pluginsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Base(path) != "plugin.json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil { return nil }
		var m struct {
			Signature string `json:"signature"`
			Binary    string `json:"binary"`
		}
		if json.Unmarshal(data, &m) != nil || m.Signature == "" || m.Binary == "" {
			return nil
		}
		binaryPath := filepath.Join(filepath.Dir(path), filepath.Base(m.Binary))
		binaryData, err := os.ReadFile(binaryPath)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: binary not found: %s", filepath.Dir(path), m.Binary))
			return nil
		}
		actual := fmt.Sprintf("sha256:%x", sha256.Sum256(binaryData))
		if actual != m.Signature {
			failures = append(failures, fmt.Sprintf("%s: signature mismatch. expected=%s actual=%s",
				filepath.Base(filepath.Dir(path)), m.Signature, actual))
		}
		return nil
	})
	if len(failures) > 0 {
		return CheckResult{Name: "plugin-signatures", Passed: false,
			Detail: strings.Join(failures, "\n")}
	}
	return CheckResult{Name: "plugin-signatures", Passed: true, Detail: "所有插件签名一致"}
}

// checkConfigSchema 配置文件结构合法性。
func checkConfigSchema(repoRoot string) CheckResult {
	examplePath := filepath.Join(repoRoot, "daemon.yaml.example")
	data, err := os.ReadFile(examplePath)
	if err != nil {
		return CheckResult{Name: "config-schema", Passed: true,
			Detail: "daemon.yaml.example 不存在（跳过 Schema 检查）"}
	}
	content := string(data)
	checks := []struct{ key, desc string }{
		{"daemon:", "缺少 [daemon:] 配置块"},
		{"autonomy_level", "缺少 autonomy_level 字段"},
		{"llm:", "缺少 [llm:] 配置块"},
		{"model:", "缺少 model 字段"},
		{"api_key:", "缺少 api_key 字段"},
		{"persona:", "缺少 persona 字段"},
	}
	var missing []string
	for _, c := range checks {
		if !strings.Contains(content, c.key) {
			missing = append(missing, c.desc)
		}
	}
	if len(missing) > 0 {
		return CheckResult{Name: "config-schema", Passed: false,
			Detail: "配置模板缺失字段: " + strings.Join(missing, ", ")}
	}
	return CheckResult{Name: "config-schema", Passed: true, Detail: "配置模板结构完整"}
}

// checkArchitectureContracts 架构契约覆盖率。
func checkArchitectureContracts(repoRoot string) CheckResult {
	// 检查 PipelineRunner check() 是否为硬编码 stub
	prPath := filepath.Join(repoRoot, "internal/scheduler/pipelinerunner.go")
	data, err := os.ReadFile(prPath)
	if err != nil {
		return CheckResult{Name: "architecture-contracts", Passed: false,
			Detail: "无法读取 pipelinerunner.go"}
	}
	content := string(data)
	// 核心契约：check() 不应硬编码返回 CheckPASS
	if strings.Contains(content, "return CheckPASS") && !strings.Contains(content, "MultiLLMVerifier") {
		return CheckResult{Name: "architecture-contracts", Passed: false,
			Detail: "pipelinerunner.go check() 硬编码返回 CheckPASS，未集成 MultiLLMVerifier"}
	}
	return CheckResult{Name: "architecture-contracts", Passed: true, Detail: "核心契约已实现"}
}
