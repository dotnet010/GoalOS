package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSkillLoader_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	loader := NewSkillLoader(dir)
	skills, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 0 {
		t.Fatalf("expected 0 skills, got %d", len(skills))
	}
}

func TestSkillLoader_NonexistentDir(t *testing.T) {
	loader := NewSkillLoader("/nonexistent/path")
	skills, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}
	if skills != nil {
		t.Fatal("expected nil for nonexistent dir")
	}
}

func TestSkillLoader_ValidSkill(t *testing.T) {
	dir := t.TempDir()
	content := `---
name: code-review
version: "1.0"
description: Review code for quality issues
author: GoalOS
tags: [quality, review]
gate_type: quality
---

# Code Review Skill

Check the code for:
- Naming conventions
- Error handling
- Concurrency safety
`
	os.WriteFile(filepath.Join(dir, "code-review.md"), []byte(content), 0644)

	loader := NewSkillLoader(dir)
	skills, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	s := skills[0]
	if s.Name != "code-review" {
		t.Fatalf("expected name='code-review', got '%s'", s.Name)
	}
	if s.Body == "" {
		t.Fatal("expected non-empty body")
	}
}

func TestSkillRegistry_Lookup(t *testing.T) {
	reg := NewSkillRegistry()
	reg.Register([]Skill{
		{Name: "code-review", GateType: "quality"},
		{Name: "security-scan", GateType: "security"},
	})

	s, ok := reg.Lookup("code-review")
	if !ok || s.Name != "code-review" {
		t.Fatal("lookup failed")
	}

	_, ok = reg.Lookup("nonexistent")
	if ok {
		t.Fatal("expected false for nonexistent skill")
	}
}

func TestSkillRegistry_ListByGateType(t *testing.T) {
	reg := NewSkillRegistry()
	reg.Register([]Skill{
		{Name: "code-review", GateType: "quality"},
		{Name: "security-scan", GateType: "security"},
		{Name: "generic-check", GateType: ""}, // matches all
	})

	quality := reg.ListByGateType("quality")
	if len(quality) != 2 { // code-review + generic-check
		t.Fatalf("expected 2 quality skills, got %d", len(quality))
	}
}
