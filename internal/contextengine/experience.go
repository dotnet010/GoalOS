// Package contextengine — 经验体系。
// 每完成一个 Goal → 自动生成 Decision Record + Lesson Record（Markdown 文件）。
// 同领域 Goal ≥ 3 → 后台 LLM 提炼 Pattern。
// 用户可查看、编辑、删除所有经验文件。
//
// 设计依据：05 架构文档 §7.3、R54。
package contextengine

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DecisionRecord 是单个 Goal 的关键决策记录。
type DecisionRecord struct {
	GoalID    string
	Title     string
	Decisions []Decision
	CreatedAt time.Time
}

// Decision 是单条决策。
type Decision struct {
	Question string // 决策问题
	Choice   string // 选择的方案
	Reason   string // 理由
	Outcome  string // 结果（正确/错误/待验证）
}

// LessonRecord 是单个 Goal 的经验教训。
type LessonRecord struct {
	GoalID     string
	Title      string
	DidWell    []string // 做对了的
	ToImprove  []string // 可改进的
	Reusable   []string // 可复用的
	CreatedAt  time.Time
}

// WriteDecision 生成 Decision Record Markdown 文件。
func (e *Engine) WriteDecision(goalID string, record *DecisionRecord) error {
	dir := filepath.Join(e.memoryDir, "decisions")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("experience: 创建 decisions 目录失败: %w", err)
	}

	path := filepath.Join(dir, goalID+"-decisions.md")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("experience: 创建 decision 文件失败: %w", err)
	}
	defer f.Close()

	fmt.Fprintf(f, "# %s — 关键决策记录\n\n", record.Title)
	fmt.Fprintf(f, "**Goal ID**: %s\n", record.GoalID)
	fmt.Fprintf(f, "**生成时间**: %s\n\n", record.CreatedAt.Format(time.RFC3339))

	for i, d := range record.Decisions {
		fmt.Fprintf(f, "## 决策 %d: %s\n\n", i+1, d.Question)
		fmt.Fprintf(f, "- **问题**: %s\n", d.Question)
		fmt.Fprintf(f, "- **决策**: %s\n", d.Choice)
		fmt.Fprintf(f, "- **理由**: %s\n", d.Reason)
		fmt.Fprintf(f, "- **结果**: %s\n\n", d.Outcome)
	}
	return nil
}

// WriteLesson 生成 Lesson Record Markdown 文件。
func (e *Engine) WriteLesson(goalID string, record *LessonRecord) error {
	dir := filepath.Join(e.memoryDir, "lessons")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("experience: 创建 lessons 目录失败: %w", err)
	}

	path := filepath.Join(dir, goalID+"-lessons.md")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("experience: 创建 lesson 文件失败: %w", err)
	}
	defer f.Close()

	fmt.Fprintf(f, "# %s — 经验教训\n\n", record.Title)
	fmt.Fprintf(f, "**Goal ID**: %s\n", record.GoalID)
	fmt.Fprintf(f, "**生成时间**: %s\n\n", record.CreatedAt.Format(time.RFC3339))

	fmt.Fprintf(f, "## 做对了的\n\n")
	for _, item := range record.DidWell {
		fmt.Fprintf(f, "- %s\n", item)
	}

	fmt.Fprintf(f, "\n## 可改进的\n\n")
	for _, item := range record.ToImprove {
		fmt.Fprintf(f, "- %s\n", item)
	}

	fmt.Fprintf(f, "\n## 下次可复用\n\n")
	for _, item := range record.Reusable {
		fmt.Fprintf(f, "- %s\n", item)
	}
	return nil
}
