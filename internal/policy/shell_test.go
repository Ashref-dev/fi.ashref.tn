package policy

import "testing"

func TestResolveShellMode(t *testing.T) {
	if mode := ResolveShellMode(false, nil); mode != ShellModeReadOnly {
		t.Fatalf("expected read-only mode, got %s", mode)
	}
	if mode := ResolveShellMode(false, []string{"git status"}); mode != ShellModeAllowlist {
		t.Fatalf("expected allowlist mode, got %s", mode)
	}
	if mode := ResolveShellMode(true, nil); mode != ShellModeUnsafe {
		t.Fatalf("expected unsafe mode, got %s", mode)
	}
}

func TestEvaluateShellCommandAllowlist(t *testing.T) {
	decision := EvaluateShellCommand("git status -sb", false, []string{"git status"})
	if !decision.Allowed {
		t.Fatalf("expected command allowed, reason: %s", decision.Reason)
	}
	blocked := EvaluateShellCommand("git commit -m x", false, []string{"git status"})
	if blocked.Allowed {
		t.Fatalf("expected command blocked")
	}
}

func TestEvaluateShellCommandReadOnly(t *testing.T) {
	decision := EvaluateShellCommand("git status", false, nil)
	if decision.Allowed {
		t.Fatalf("expected command blocked in read-only mode")
	}
}

func TestEvaluateShellCommandNetworkBlocked(t *testing.T) {
	decision := EvaluateShellCommand("curl https://example.com", false, []string{"curl"})
	if decision.Allowed {
		t.Fatalf("expected curl blocked in allowlist mode")
	}
}
