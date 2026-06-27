// Package skills 实现 GoalOS Skill 体系（agentskills.io 标准）。
// v0.1.0 最小实现：SkillLoader + SkillRegistry + SkillGate 类型。
// v1.2: 用户可见管理（goalos skill list）。
// v1.5: 市场对接 + merge/parallel/serial 执行模式。
//
// 设计依据：05 架构 §4.4、R-351。
package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Skill 是声明式指令包（Markdown + YAML Frontmatter）。
// 遵循 agentskills.io 开放标准。
type Skill struct {
	Name        string   `yaml:"name"`        // 唯一标识
	Version     string   `yaml:"version"`     // 语义化版本
	Description string   `yaml:"description"` // 一句话描述
	Author      string   `yaml:"author"`      // 作者
	Tags        []string `yaml:"tags"`        // 标签
	GateType    string   `yaml:"gate_type"`   // 适用的 Gate 类型（如 "quality", "security"）
	// Body 是 Markdown 正文（YAML Frontmatter 之后的内容）。不包含在此结构中。
	Body string `yaml:"-"`
}

// SkillLoader 扫描 ~/.goalos/skills/ 目录，加载并验证 Skill 文件。
type SkillLoader struct {
	skillsDir string
}

// NewSkillLoader 创建 SkillLoader。
func NewSkillLoader(skillsDir string) *SkillLoader {
	return &SkillLoader{skillsDir: skillsDir}
}

// Load 扫描 skills 目录，返回所有有效 Skill。
func (l *SkillLoader) Load() ([]Skill, error) {
	if _, err := os.Stat(l.skillsDir); os.IsNotExist(err) {
		return nil, nil // 目录不存在 → 无 Skill，非错误
	}

	entries, err := os.ReadDir(l.skillsDir)
	if err != nil {
		return nil, fmt.Errorf("skills: read dir %s: %w", l.skillsDir, err)
	}

	var skills []Skill
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		skill, err := l.parseFile(filepath.Join(l.skillsDir, entry.Name()))
		if err != nil {
			continue // 跳过无效 Skill 文件
		}
		skills = append(skills, *skill)
	}
	return skills, nil
}

// parseFile 解析单个 Skill 文件（YAML Frontmatter + Markdown Body）。
func (l *SkillLoader) parseFile(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	content := string(data)
	// 提取 YAML Frontmatter（--- 分隔符）
	if !strings.HasPrefix(content, "---\n") {
		return nil, fmt.Errorf("skills: %s: missing YAML frontmatter", path)
	}

	parts := strings.SplitN(content[4:], "\n---\n", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("skills: %s: invalid frontmatter", path)
	}

	var skill Skill
	if err := yaml.Unmarshal([]byte(parts[0]), &skill); err != nil {
		return nil, fmt.Errorf("skills: %s: %w", path, err)
	}
	skill.Body = strings.TrimSpace(parts[1])

	if skill.Name == "" {
		return nil, fmt.Errorf("skills: %s: name is required", path)
	}
	return &skill, nil
}

// SkillRegistry 按名称查找已加载的 Skill。
type SkillRegistry struct {
	skills map[string]Skill
}

// NewSkillRegistry 创建 SkillRegistry。
func NewSkillRegistry() *SkillRegistry {
	return &SkillRegistry{skills: make(map[string]Skill)}
}

// Register 注册一个 Skill 列表。
func (r *SkillRegistry) Register(skills []Skill) {
	for _, s := range skills {
		r.skills[s.Name] = s
	}
}

// Lookup 按名称查找 Skill。
func (r *SkillRegistry) Lookup(name string) (Skill, bool) {
	s, ok := r.skills[name]
	return s, ok
}

// ListByGateType 返回适用于指定 Gate 类型的所有 Skill。
func (r *SkillRegistry) ListByGateType(gateType string) []Skill {
	var result []Skill
	for _, s := range r.skills {
		if s.GateType == gateType || s.GateType == "" {
			result = append(result, s)
		}
	}
	return result
}

// Count 返回已注册 Skill 数量。
func (r *SkillRegistry) Count() int {
	return len(r.skills)
}
