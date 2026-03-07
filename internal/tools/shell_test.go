package tools

import (
	"context"
	"encoding/json"
	"testing"

	"fi-cli/internal/policy"
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
	allowed := policy.EvaluateShellCommand("git status -sb", false, tool.allowlist)
	if !allowed.Allowed {
		t.Fatalf("expected git status prefix to be allowed: %s", allowed.Reason)
	}
	blocked := policy.EvaluateShellCommand("git commit -m test", false, tool.allowlist)
	if blocked.Allowed {
		t.Fatalf("expected git commit to be blocked by allowlist")
	}
}
