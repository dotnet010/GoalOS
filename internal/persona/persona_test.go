package persona_test

import (
	"strings"
	"testing"

	"github.com/goalos/goalos/internal/persona"
)

func TestConcise_GoalCreated(t *testing.T) {
	msg := persona.Concise.Render("GoalCreated", map[string]interface{}{"title": "开发CRM"})
	if !strings.Contains(msg, "开发CRM") {
		t.Errorf("expected message to contain goal title, got: %s", msg)
	}
}

func TestConcise_ActionPendingApproval(t *testing.T) {
	msg := persona.Concise.Render("ActionPendingApproval", map[string]interface{}{
		"action_description": "删除生产数据库",
		"risk_level":         "L4",
	})
	if !strings.Contains(msg, "L4") {
		t.Errorf("risk level should be in message: %s", msg)
	}
	if !strings.Contains(msg, "删除生产数据库") {
		t.Errorf("action description should be in message: %s", msg)
	}
	if !strings.Contains(msg, "批准") || !strings.Contains(msg, "拒绝") {
		t.Errorf("approval options should be in message: %s", msg)
	}
}

func TestAllBuiltins(t *testing.T) {
	builtins := persona.Builtin()
	if len(builtins) != 3 {
		t.Errorf("expected 3 builtins, got %d", len(builtins))
	}
	names := map[string]bool{}
	for _, p := range builtins {
		names[p.Name] = true
	}
	for _, name := range []string{"concise", "warm", "minimal"} {
		if !names[name] {
			t.Errorf("missing builtin: %s", name)
		}
	}
}

func TestGet_DefaultFallback(t *testing.T) {
	p := persona.Get("nonexistent")
	if p.Name != "concise" {
		t.Errorf("expected concise fallback, got %s", p.Name)
	}
}

func TestMinimal_NoEmoji(t *testing.T) {
	msg := persona.Minimal.Render("GoalCreated", map[string]interface{}{"title": "test"})
	if strings.Contains(msg, "✅") || strings.Contains(msg, "🎉") {
		t.Errorf("minimal persona should not use emoji: %s", msg)
	}
}
