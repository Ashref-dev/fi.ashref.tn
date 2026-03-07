package config

import "testing"

func TestLoadDefaultsToolCallCaps(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("FICLI_API_KEY", "")
	t.Setenv("FICLI_MODEL", "")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.ToolLimits.GrepMaxCalls != 30 {
		t.Fatalf("expected grep max calls 30, got %d", cfg.ToolLimits.GrepMaxCalls)
	}
	if cfg.ToolLimits.ShellMaxCalls != 30 {
		t.Fatalf("expected shell max calls 30, got %d", cfg.ToolLimits.ShellMaxCalls)
	}
	if cfg.ToolLimits.WebMaxCalls != 30 {
		t.Fatalf("expected web max calls 30, got %d", cfg.ToolLimits.WebMaxCalls)
	}
}
