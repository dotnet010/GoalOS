package contextengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goalos/goalos/internal/contextengine"
)

func TestBuildPageTable(t *testing.T) {
	dir := t.TempDir()
	goalDir := filepath.Join(dir, "goal_001")
	os.MkdirAll(goalDir, 0755)

	// 创建带 Frontmatter 的 Markdown 文件
	content := `---
title: "CRM系统架构设计"
summary: "三层架构：React+Go+PostgreSQL"
keywords: [架构, REST API, 前后端通信]
---
# CRM系统架构设计
## 前端架构
React SPA。WebSocket 实时同步。
## 后端架构
Go REST API。分层架构。`
	os.WriteFile(filepath.Join(goalDir, "架构设计.md"), []byte(content), 0644)

	engine := contextengine.New(dir, "")
	pt, err := engine.BuildPageTable("goal_001")
	if err != nil {
		t.Fatalf("BuildPageTable failed: %v", err)
	}
	if len(pt.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(pt.Files))
	}

	f := pt.Files[0]
	if f.Title != "CRM系统架构设计" {
		t.Errorf("expected title 'CRM系统架构设计', got '%s'", f.Title)
	}
	if len(f.Keywords) != 3 {
		t.Errorf("expected 3 keywords, got %d", len(f.Keywords))
	}
	if len(f.Sections) < 2 {
		t.Errorf("expected at least 2 sections, got %d", len(f.Sections))
	}
}

func TestSearch(t *testing.T) {
	dir := t.TempDir()
	goalDir := filepath.Join(dir, "goal_search")
	os.MkdirAll(goalDir, 0755)

	os.WriteFile(filepath.Join(goalDir, "doc1.md"), []byte(`---
title: "API文档"
summary: "REST API接口规范"
keywords: [api, rest]
---
# API文档`), 0644)

	os.WriteFile(filepath.Join(goalDir, "doc2.md"), []byte(`---
title: "部署指南"
summary: "Docker+Kubernetes部署"
keywords: [docker, k8s]
---
# 部署指南`), 0644)

	engine := contextengine.New(dir, "")
	engine.BuildPageTable("goal_search")

	results, err := engine.Search("goal_search", "docker")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'docker', got %d", len(results))
	}
	if results[0].Title != "部署指南" {
		t.Errorf("expected '部署指南', got '%s'", results[0].Title)
	}
}

func TestEmptyDir(t *testing.T) {
	dir := t.TempDir()
	engine := contextengine.New(dir, "")
	pt, err := engine.BuildPageTable("nonexistent")
	if err != nil {
		t.Fatalf("BuildPageTable should not error on empty dir: %v", err)
	}
	if len(pt.Files) != 0 {
		t.Errorf("expected 0 files, got %d", len(pt.Files))
	}
}
