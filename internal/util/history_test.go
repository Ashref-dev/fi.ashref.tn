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
