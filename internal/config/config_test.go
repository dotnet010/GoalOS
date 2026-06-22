package config_test

import (
	"os"
	"testing"

	"github.com/goalos/goalos/internal/config"
)

func TestDefaultConfig(t *testing.T) {
	cfg := config.Default()
	if cfg.Daemon.Port != 18920 {
		t.Errorf("expected port 18920, got %d", cfg.Daemon.Port)
	}
	if cfg.Daemon.AutonomyLevel != "autonomous" {
		t.Errorf("expected approve, got %s", cfg.Daemon.AutonomyLevel)
	}
	if cfg.Persona != "concise" {
		t.Errorf("expected concise, got %s", cfg.Persona)
	}
	if cfg.LLM.Provider != "anthropic" {
		t.Errorf("expected anthropic, got %s", cfg.LLM.Provider)
	}
}

func TestEnvOverride(t *testing.T) {
	os.Setenv("GOALOS_PORT", "19999")
	os.Setenv("GOALOS_PERSONA", "warm")
	defer func() {
		os.Unsetenv("GOALOS_PORT")
		os.Unsetenv("GOALOS_PERSONA")
	}()

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Daemon.Port != 19999 {
		t.Errorf("env override: expected port 19999, got %d", cfg.Daemon.Port)
	}
	if cfg.Persona != "warm" {
		t.Errorf("env override: expected warm, got %s", cfg.Persona)
	}
	// Non-overridden values should stay at defaults
	if cfg.Daemon.AutonomyLevel != "autonomous" {
		t.Errorf("expected default approve, got %s", cfg.Daemon.AutonomyLevel)
	}
}

func TestLoadNonExistent(t *testing.T) {
	cfg, err := config.Load("/nonexistent/path")
	if err != nil {
		t.Fatalf("Load should succeed with defaults: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
}
