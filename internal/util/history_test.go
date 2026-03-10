package util

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadShellHistory(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, ".zsh_history")
	content := strings.Join([]string{
		": 1680000000:0;echo hello",
		": 1680000001:0;API_KEY=secretvalue",
		"- cmd: ls -la",
		"plain command",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write history: %v", err)
	}
	old := os.Getenv("HISTFILE")
	_ = os.Setenv("HISTFILE", path)
	defer func() {
		_ = os.Setenv("HISTFILE", old)
	}()

	lines := LoadShellHistory(10)
	if len(lines) == 0 {
		t.Fatalf("expected history lines")
	}
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "secretvalue") {
		t.Fatalf("expected redaction")
	}
	if !strings.Contains(joined, "echo hello") {
		t.Fatalf("expected normalized history")
	}
}

func TestLoadShellHistoryTailLargeFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, ".zsh_history")
	var b strings.Builder
	for i := 0; i < 5000; i++ {
		b.WriteString(": 1680000000:0;echo line")
		b.WriteString(strings.TrimSpace(strings.Repeat("x", 20)))
		b.WriteString("\n")
	}
	b.WriteString(": 1680005000:0;tmux ls\n")
	b.WriteString(": 1680005001:0;echo final\n")
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatalf("write history: %v", err)
	}
	old := os.Getenv("HISTFILE")
	_ = os.Setenv("HISTFILE", path)
	defer func() {
		_ = os.Setenv("HISTFILE", old)
	}()

	lines := LoadShellHistory(2)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0] != "tmux ls" || lines[1] != "echo final" {
		t.Fatalf("unexpected tail lines: %#v", lines)
	}
}
