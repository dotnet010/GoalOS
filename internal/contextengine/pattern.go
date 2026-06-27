// Package contextengine — Pattern 提取（v0.1.0 H10）。
// 同领域 >=3 个 Goal 完成后触发通用模式提炼。
package contextengine

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/goalos/goalos/pkg/events"
)

type domainPattern struct {
	count int
	goals []string
}

// ExtractPattern 在同领域 >=3 个 Goal 完成时提炼通用模式（v0.1.0 H10）。
func (e *Engine) ExtractPattern(goalID string, goalText string) {
	domain := inferDomain(goalText)
	e.mu.Lock()
	if e.domainCounts == nil {
		e.domainCounts = make(map[string]*domainPattern)
	}
	dp, ok := e.domainCounts[domain]
	if !ok {
		dp = &domainPattern{}
		e.domainCounts[domain] = dp
	}
	dp.count++
	dp.goals = append(dp.goals, goalID)
	count := dp.count
	goals := make([]string, len(dp.goals))
	copy(goals, dp.goals)
	e.mu.Unlock()

	if count < 3 {
		return
	}
	log.Printf("[ContextEngine] pattern extraction: domain=%s count=%d", domain, count)
	e.writePattern(domain, goals)
	if e.bus != nil {
		e.bus.Publish(events.Event{
			Type:   events.TypePatternExtracted,
			GoalID: goalID,
			Source: "context-engine",
			Payload: map[string]interface{}{
				"domain": domain, "pattern_count": float64(count),
			},
		})
	}
}

func inferDomain(goal string) string {
	g := strings.ToLower(goal)
	switch {
	case strings.Contains(g, "代码") || strings.Contains(g, "code") || strings.Contains(g, "开发") || strings.Contains(g, "编程"):
		return "code-generation"
	case strings.Contains(g, "数据") || strings.Contains(g, "分析") || strings.Contains(g, "data"):
		return "data-analysis"
	case strings.Contains(g, "调研") || strings.Contains(g, "研究") || strings.Contains(g, "research"):
		return "research"
	case strings.Contains(g, "写") || strings.Contains(g, "创作") || strings.Contains(g, "内容") || strings.Contains(g, "文章"):
		return "content-creation"
	default:
		return "general"
	}
}

func (e *Engine) writePattern(domain string, goalIDs []string) {
	dir := filepath.Join(e.memoryDir, "patterns")
	os.MkdirAll(dir, 0755)
	f, err := os.Create(filepath.Join(dir, domain+"-pattern.md"))
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "# %s — 通用模式\n\n**%d 个 Goal 完成**\n\n**Goal IDs**: %s\n",
		domain, len(goalIDs), strings.Join(goalIDs, ", "))
}
