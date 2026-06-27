// Package contextengine 实现 GoalOS Context Engine——分层上下文管理。
// 二层已实现：PageTable（Frontmatter 提取 + 关键词索引）+ Experience（决策/经验文件生成）。
// 四层 v1.5 预留：Immutable / Working Summary / Active Page / Input。
//
// 设计依据：05 架构文档 §7、R52-R55、R-360。
package contextengine

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/pkg/events"
)

// Engine 是 Context Engine。
type Engine struct {
	goalsDir        string
	memoryDir       string
	bus             *eventbus.EventBus
	pageTableCache  map[string]*PageTable
	domainCounts    map[string]*domainPattern // v0.1.0 H10: 领域计数
	mu              sync.RWMutex
}

// New 创建 Context Engine。
func New(goalsDir, memoryDir string) *Engine {
	return &Engine{
		goalsDir:       goalsDir,
		memoryDir:      memoryDir,
		pageTableCache: make(map[string]*PageTable),
	}
}

// PageTable 是 Goal 的文件分层摘要索引。
// 从所有文件的 Frontmatter 聚合生成。纯缓存——可删除重建。
type PageTable struct {
	GoalID string
	Files  []FileEntry
}

// FileEntry 是单个文件的分层摘要。
type FileEntry struct {
	Path        string
	Title       string
	Summary     string   // 文件级摘要（50-200字）
	Keywords    []string // 关键词列表
	Sections    []SectionEntry
}

// SectionEntry 是章节摘要。
type SectionEntry struct {
	Heading string // 章节标题
	Summary string // 章节摘要
}

// BuildPageTable 扫描 Goal 目录下所有 Markdown 文件，提取 Frontmatter 摘要。
// 聚合生成 PageTable。保存在缓存中。
func (e *Engine) BuildPageTable(goalID string) (*PageTable, error) {
	goalDir := filepath.Join(e.goalsDir, goalID)
	pt := &PageTable{GoalID: goalID}

	err := filepath.Walk(goalDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		// 仅处理 Markdown 文件
		if !strings.HasSuffix(path, ".md") {
			return nil
		}

		entry, err := extractFrontmatter(path)
		if err != nil {
			return nil // 跳过无法解析的文件
		}
		entry.Path = path
		pt.Files = append(pt.Files, *entry)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("contextengine: 扫描目录失败: %w", err)
	}

	e.pageTableCache[goalID] = pt
	return pt, nil
}

// GetPageTable 获取 Goal 的 PageTable。优先从缓存读取。
func (e *Engine) GetPageTable(goalID string) (*PageTable, error) {
	if pt, ok := e.pageTableCache[goalID]; ok {
		return pt, nil
	}
	return e.BuildPageTable(goalID)
}

// extractFrontmatter 从 Markdown 文件中提取 YAML Frontmatter。
// 格式：第一行必须为 "---"，结束时为下一个 "---"。
func extractFrontmatter(path string) (*FileEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	entry := &FileEntry{}
	scanner := bufio.NewScanner(f)

	// 检查第一行是否为 "---"
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return entry, nil // 无 Frontmatter——返回空 entry
	}

	// 解析 Frontmatter
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "---" {
			break // Frontmatter 结束
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, `"'`) // 去除引号

		switch key {
		case "title":
			entry.Title = val
		case "summary":
			entry.Summary = val
		case "keywords":
			// 解析 YAML 列表: [kw1, kw2, ...]
			val = strings.Trim(val, "[]")
			for _, kw := range strings.Split(val, ",") {
				kw = strings.TrimSpace(kw)
				if kw != "" {
					entry.Keywords = append(entry.Keywords, kw)
				}
			}
		}
	}

	// 提取章节标题作为 Sections
	// 重新扫描文件寻找 "## " 标题
	f.Seek(0, 0)
	scanner = bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "## ") {
			heading := strings.TrimPrefix(line, "## ")
			entry.Sections = append(entry.Sections, SectionEntry{
				Heading: heading,
			})
		}
	}

	return entry, nil
}

// Search 在 PageTable 中搜索关键词。返回匹配的文件路径列表。
func (e *Engine) Search(goalID, query string) ([]FileEntry, error) {
	pt, err := e.GetPageTable(goalID)
	if err != nil {
		return nil, err
	}

	var results []FileEntry
	query = strings.ToLower(query)
	for _, f := range pt.Files {
		if strings.Contains(strings.ToLower(f.Title), query) ||
			strings.Contains(strings.ToLower(f.Summary), query) {
			results = append(results, f)
			continue
		}
		for _, kw := range f.Keywords {
			if strings.Contains(strings.ToLower(kw), query) {
				results = append(results, f)
				break
			}
		}
	}
	return results, nil
}

// Start subscribes to Goal lifecycle events and begins processing（v0.1.1: R-362 wiring fix）。
func (e *Engine) Start(bus *eventbus.EventBus) {
	e.bus = bus
	e.bus.Subscribe(events.TypeGoalCompleted, e.onGoalCompleted)
	e.bus.Subscribe(events.TypeGoalFailed, e.onGoalFailed)
	log.Println("[ContextEngine] started, subscribed to GoalCompleted/GoalFailed")
}

// onGoalCompleted 在 Goal 完成时生成经验文件。
func (e *Engine) onGoalCompleted(evt events.Event) error {
	goalID := evt.GoalID
	log.Printf("[ContextEngine] generating experience for completed goal: %s", goalID)
	e.WriteDecision(goalID, &DecisionRecord{GoalID: goalID, Title: "Goal completed"})
	e.WriteLesson(goalID, &LessonRecord{GoalID: goalID, Title: "Goal execution completed"})
	// v0.1.0 H10: 跨 Goal 模式提炼
	if goalText, ok := evt.Payload["goal_text"].(string); ok {
		e.ExtractPattern(goalID, goalText)
	}
	return nil
}

// onGoalFailed 在 Goal 失败时生成经验文件（记录失败原因）。
func (e *Engine) onGoalFailed(evt events.Event) error {
	goalID := evt.GoalID
	reason := ""
	if r, ok := evt.Payload["error"].(string); ok {
		reason = r
	}
	log.Printf("[ContextEngine] generating lessons for failed goal: %s (%s)", goalID, reason)
	e.WriteLesson(goalID, &LessonRecord{GoalID: goalID, Title: "Goal failed: " + reason})
	return nil
}

// AssembleContext 为 Agent 调用组装结构化上下文（v0.1.1: R-362）。
func (e *Engine) AssembleContext(goalID string, userInput string) *AgentContext {
	e.mu.RLock()
	pt := e.pageTableCache[goalID]
	e.mu.RUnlock()

	return &AgentContext{
		GoalID:    goalID,
		PageTable: pt,
		UserInput: userInput,
	}
}

// AgentContext 是 Agent 调用时的上下文（v0.1.1）。
type AgentContext struct {
	GoalID    string
	PageTable *PageTable
	UserInput string
}
