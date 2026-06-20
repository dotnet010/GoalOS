// ContextEngine 集成测试——验证经验文件生成。
// 设计依据：R238（错误路径）、R239。
package contextengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goalos/goalos/internal/contextengine"
)

func TestWriteDecision(t *testing.T) {
	dir := t.TempDir()
	engine := contextengine.New("", dir)

	record := &contextengine.DecisionRecord{
		GoalID: "goal_001",
		Title:  "CRM系统",
		Decisions: []contextengine.Decision{
			{Question: "REST vs GraphQL", Choice: "REST", Reason: "团队熟悉", Outcome: "正确"},
		},
		CreatedAt: time.Now(),
	}

	if err := engine.WriteDecision("goal_001", record); err != nil {
		t.Fatalf("WriteDecision failed: %v", err)
	}

	// 验证文件存在且内容正确
	path := filepath.Join(dir, "decisions", "goal_001-decisions.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 decision 文件失败: %v", err)
	}
	if !strings.Contains(string(data), "CRM系统") {
		t.Error("decision 文件缺少标题")
	}
	if !strings.Contains(string(data), "REST vs GraphQL") {
		t.Error("decision 文件缺少决策内容")
	}
}

func TestWriteLesson(t *testing.T) {
	dir := t.TempDir()
	engine := contextengine.New("", dir)

	record := &contextengine.LessonRecord{
		GoalID:    "goal_001",
		Title:     "CRM系统",
		DidWell:   []string{"需求分析充分"},
		ToImprove: []string{"数据库索引不足"},
		Reusable:  []string{"用户认证模块"},
		CreatedAt: time.Now(),
	}

	if err := engine.WriteLesson("goal_001", record); err != nil {
		t.Fatalf("WriteLesson failed: %v", err)
	}

	path := filepath.Join(dir, "lessons", "goal_001-lessons.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 lesson 文件失败: %v", err)
	}
	if !strings.Contains(string(data), "需求分析充分") {
		t.Error("lesson 文件缺少 DidWell 内容")
	}
	if !strings.Contains(string(data), "数据库索引不足") {
		t.Error("lesson 文件缺少 ToImprove 内容")
	}
}
