package tools

import (
	"context"
	"encoding/json"
	"testing"
)

func TestShellToolBlocksDestructive(t *testing.T) {
	tool := NewShellTool()
	input, _ := json.Marshal(map[string]any{"command": "rm -rf /"})
	_, err := tool.Execute(context.Background(), input, Meta{RepoRoot: ".", UnsafeShell: false, ToolTimeoutSeconds: 1, MaxBytes: 1024})
	if err == nil {
		t.Fatalf("expected destructive command to be blocked")
	}
}

func TestShellToolBlocksNetwork(t *testing.T) {
	tool := NewShellTool()
	input, _ := json.Marshal(map[string]any{"command": "curl https://example.com"})
	_, err := tool.Execute(context.Background(), input, Meta{RepoRoot: ".", UnsafeShell: false, ToolTimeoutSeconds: 1, MaxBytes: 1024})
	if err == nil {
		t.Fatalf("expected network command to be blocked")
	}
}

func TestShellToolBlocksUnknown(t *testing.T) {
	tool := NewShellTool()
	input, _ := json.Marshal(map[string]any{"command": "notacmd --help"})
	_, err := tool.Execute(context.Background(), input, Meta{RepoRoot: ".", UnsafeShell: false, ToolTimeoutSeconds: 1, MaxBytes: 1024})
	if err == nil {
		t.Fatalf("expected unknown command to be blocked")
	}
}
