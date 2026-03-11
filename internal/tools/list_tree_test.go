package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListTreeDepthCapAddsEllipsis(t *testing.T) {
	repoRoot := t.TempDir()
	mustWriteFile(t, filepath.Join(repoRoot, "a", "b", "c", "d", "file.txt"), "x")

	tool := NewListTreeTool()
	input, _ := json.Marshal(map[string]any{"path": ".", "max_depth": 2, "max_entries": 100})
	res, err := tool.Execute(context.Background(), input, Meta{RepoRoot: repoRoot, ToolTimeoutSeconds: 2, MaxBytes: 16 * 1024})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	out, ok := res.Payload.(listTreeOutput)
	if !ok {
		t.Fatalf("unexpected payload type: %T", res.Payload)
	}
	joined := strings.Join(out.Lines, "\n")
	if !strings.Contains(joined, "...") {
		t.Fatalf("expected depth truncation marker, got %s", joined)
	}
	if !out.Truncated {
		t.Fatalf("expected truncated=true for depth cap")
	}
}

func TestListTreeIgnoresCommonHeavyDirs(t *testing.T) {
	repoRoot := t.TempDir()
	mustWriteFile(t, filepath.Join(repoRoot, "node_modules", "left-pad", "index.js"), "module.exports = 1")
	mustWriteFile(t, filepath.Join(repoRoot, ".git", "config"), "[core]")
	mustWriteFile(t, filepath.Join(repoRoot, "src", "main.go"), "package main")

	tool := NewListTreeTool()
	res, err := tool.Execute(context.Background(), nil, Meta{RepoRoot: repoRoot, ToolTimeoutSeconds: 2, MaxBytes: 16 * 1024})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	out := res.Payload.(listTreeOutput)
	joined := strings.Join(out.Lines, "\n")
	if strings.Contains(joined, "node_modules") {
		t.Fatalf("expected node_modules to be ignored, got %s", joined)
	}
	if strings.Contains(joined, ".git") {
		t.Fatalf("expected .git to be ignored, got %s", joined)
	}
	if !strings.Contains(joined, "src/") {
		t.Fatalf("expected src to be present, got %s", joined)
	}
}

func TestListTreeRejectsPathTraversal(t *testing.T) {
	repoRoot := t.TempDir()
	tool := NewListTreeTool()
	input, _ := json.Marshal(map[string]any{"path": "../"})
	_, err := tool.Execute(context.Background(), input, Meta{RepoRoot: repoRoot, ToolTimeoutSeconds: 2})
	if err == nil {
		t.Fatalf("expected path traversal error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "repo root") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListTreeSupportsDrillDownPath(t *testing.T) {
	repoRoot := t.TempDir()
	mustWriteFile(t, filepath.Join(repoRoot, "src", "internal", "main.go"), "package main")

	tool := NewListTreeTool()
	input, _ := json.Marshal(map[string]any{"path": "src", "max_depth": 3, "max_entries": 100})
	res, err := tool.Execute(context.Background(), input, Meta{RepoRoot: repoRoot, ToolTimeoutSeconds: 2, MaxBytes: 16 * 1024})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	out := res.Payload.(listTreeOutput)
	if out.Path != "src" {
		t.Fatalf("expected path src, got %s", out.Path)
	}
	joined := strings.Join(out.Lines, "\n")
	if !strings.HasPrefix(joined, "src/") {
		t.Fatalf("expected src root line, got %s", joined)
	}
	if !strings.Contains(joined, "internal/") {
		t.Fatalf("expected internal directory in output, got %s", joined)
	}
}

func mustWriteFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
}
