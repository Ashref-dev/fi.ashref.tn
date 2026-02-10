package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGrepFallback(t *testing.T) {
	repoRoot := t.TempDir()
	filePath := filepath.Join(repoRoot, "sample.txt")
	if err := os.WriteFile(filePath, []byte("hello FICLI world\n"), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	tool := NewGrepTool()
	tool.rgPath = ""
	input, _ := json.Marshal(map[string]any{"pattern": "FICLI"})
	res, err := tool.Execute(context.Background(), input, Meta{RepoRoot: repoRoot, ToolTimeoutSeconds: 2, MaxResults: 10, MaxBytes: 1024})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, ok := res.Payload.(grepOutput)
	if !ok {
		t.Fatalf("unexpected payload type")
	}
	if len(out.Matches) == 0 {
		t.Fatalf("expected matches")
	}
}
