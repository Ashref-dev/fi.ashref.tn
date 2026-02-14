package tools

import (
	"context"
	"encoding/json"
	"testing"
)

func TestShellToolBlocksDestructive(t *testing.T) {
	tool := NewShellTool([]string{"rm"})
	input, _ := json.Marshal(map[string]any{"command": "rm -rf /"})
	_, err := tool.Execute(context.Background(), input, Meta{RepoRoot: ".", UnsafeShell: false, ToolTimeoutSeconds: 1, MaxBytes: 1024})
	if err == nil {
		t.Fatalf("expected destructive command to be blocked")
	}
}

func TestShellToolBlocksNetwork(t *testing.T) {
	tool := NewShellTool([]string{"curl"})
	input, _ := json.Marshal(map[string]any{"command": "curl https://example.com"})
	_, err := tool.Execute(context.Background(), input, Meta{RepoRoot: ".", UnsafeShell: false, ToolTimeoutSeconds: 1, MaxBytes: 1024})
	if err == nil {
		t.Fatalf("expected network command to be blocked")
	}
}

func TestShellToolBlocksUnknown(t *testing.T) {
	tool := NewShellTool([]string{"git"})
	input, _ := json.Marshal(map[string]any{"command": "notacmd --help"})
	_, err := tool.Execute(context.Background(), input, Meta{RepoRoot: ".", UnsafeShell: false, ToolTimeoutSeconds: 1, MaxBytes: 1024})
	if err == nil {
		t.Fatalf("expected unknown command to be blocked")
	}
}

func TestShellToolAllowlistPrefix(t *testing.T) {
	tool := NewShellTool([]string{"git status"})
	cmdParts, err := splitCommand("git status -sb")
	if err != nil {
		t.Fatalf("split failed: %v", err)
	}
	if !tool.allowed(cmdParts) {
		t.Fatalf("expected git status prefix to be allowed")
	}

	cmdParts, err = splitCommand("git commit -m test")
	if err != nil {
		t.Fatalf("split failed: %v", err)
	}
	if tool.allowed(cmdParts) {
		t.Fatalf("expected git commit to be blocked by allowlist")
	}
}
