package review

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReviewInstructionsAndPathRules(t *testing.T) {
	repoRoot := t.TempDir()
	fiDir := filepath.Join(repoRoot, ".fi")
	if err := os.MkdirAll(fiDir, 0o755); err != nil {
		t.Fatalf("failed to create .fi directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fiDir, "review.md"), []byte("Check migrations and API compatibility."), 0o644); err != nil {
		t.Fatalf("failed to write review.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fiDir, "review-paths.yaml"), []byte("rules:\n  - glob: internal/*/*.go\n    instructions: Focus on concurrency.\n  - glob: cmd/*/*.go\n    instructions: Verify CLI ergonomics.\n"), 0o644); err != nil {
		t.Fatalf("failed to write review-paths.yaml: %v", err)
	}

	instructions, err := loadReviewInstructions(repoRoot, ".fi/review.md", ".fi/review.md")
	if err != nil {
		t.Fatalf("loadReviewInstructions failed: %v", err)
	}
	if instructions != "Check migrations and API compatibility." {
		t.Fatalf("unexpected instructions: %q", instructions)
	}

	rules, err := loadPathRules(repoRoot, ".fi/review-paths.yaml", ".fi/review-paths.yaml")
	if err != nil {
		t.Fatalf("loadPathRules failed: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 path rules, got %d", len(rules))
	}

	resolved := resolvePathInstructions(rules, "internal/core/review.go")
	if resolved != "Focus on concurrency." {
		t.Fatalf("unexpected path instructions: %q", resolved)
	}
	if got := resolvePathInstructions(rules, "docs/readme.md"); got != "" {
		t.Fatalf("expected no path instructions for docs file, got %q", got)
	}
}

func TestLoadReviewDefaultsMissingFilesAreOptional(t *testing.T) {
	repoRoot := t.TempDir()
	instructions, err := loadReviewInstructions(repoRoot, ".fi/review.md", ".fi/review.md")
	if err != nil {
		t.Fatalf("expected missing default review.md to be optional, got %v", err)
	}
	if instructions != "" {
		t.Fatalf("expected empty instructions for missing default file, got %q", instructions)
	}

	rules, err := loadPathRules(repoRoot, ".fi/review-paths.yaml", ".fi/review-paths.yaml")
	if err != nil {
		t.Fatalf("expected missing default review-paths.yaml to be optional, got %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("expected no rules, got %d", len(rules))
	}
}
