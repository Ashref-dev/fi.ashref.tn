package config

import (
	"testing"

	"github.com/spf13/cobra"
)

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

func TestLoadPlanAndTimingsFlags(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cmd := &cobra.Command{Use: "fi-cli"}
	cmd.Flags().Bool("plan", false, "")
	cmd.Flags().Bool("no-plan", true, "")
	cmd.Flags().Bool("timings", false, "")
	cmd.Flags().String("model", DefaultModel, "")
	cmd.Flags().Int("max-steps", DefaultMaxSteps, "")
	cmd.Flags().String("repo", ".", "")
	cmd.Flags().String("timeout", DefaultTimeout.String(), "")
	cmd.Flags().Bool("unsafe-shell", false, "")
	cmd.Flags().Bool("no-web", false, "")
	cmd.Flags().Bool("show-header", false, "")
	cmd.Flags().Bool("show-tools", true, "")
	cmd.Flags().Bool("no-tools", false, "")
	cmd.Flags().String("mode", DefaultResponseMode, "")
	cmd.Flags().Bool("quiet", false, "")
	cmd.Flags().Bool("json", false, "")
	cmd.Flags().Bool("verbose", false, "")
	cmd.Flags().String("log-file", "", "")
	cmd.Flags().Int("history-lines", 50, "")
	cmd.Flags().Bool("no-history", false, "")
	cmd.Flags().StringSlice("shell-allow", nil, "")

	if err := cmd.Flags().Set("plan", "true"); err != nil {
		t.Fatalf("failed to set --plan: %v", err)
	}
	if err := cmd.Flags().Set("timings", "true"); err != nil {
		t.Fatalf("failed to set --timings: %v", err)
	}

	cfg, err := Load(cmd)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.NoPlan {
		t.Fatalf("expected --plan to disable no-plan mode")
	}
	if !cfg.Timings {
		t.Fatalf("expected --timings to be enabled")
	}
}
