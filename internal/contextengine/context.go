// Package contextengine 实现 GoalOS Context Engine——分层上下文管理。
// 六层上下文包：Immutable/Working Summary/Active Page/Page Table/Experience/Input。
// 管理 Frontmatter 摘要提取、Page Table 生成、经验文件检索。
//
// 设计依据：05 架构文档 §7、R52-R55。
package contextengine

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Engine 是 Context Engine。
type Engine struct {
	goalsDir    string // ~/Goals/
	memoryDir   string // ~/.goalos/memory/
	pageTableCache map[string]*PageTable // goalID → 文件索引缓存
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
