package review

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fi-cli/internal/config"
	"fi-cli/internal/llm"

	"go.uber.org/zap"
)

func TestReviewerResolveRefsAutoDetectsMain(t *testing.T) {
	repoRoot := initReviewRepo(t, "main")
	writeReviewFile(t, repoRoot, "app.txt", []byte("base\n"))
	commitReviewAll(t, repoRoot, "initial")

	reviewer := NewReviewer(llm.NewMockClient(), zap.NewNop(), testReviewConfig())
	baseRef, headRef, err := reviewer.resolveRefs(context.Background(), repoRoot, Params{})
	if err != nil {
		t.Fatalf("resolveRefs failed: %v", err)
	}
	if baseRef != "main" {
		t.Fatalf("expected base ref main, got %q", baseRef)
	}
	if headRef != "HEAD" {
		t.Fatalf("expected head ref HEAD, got %q", headRef)
	}
}

func TestReviewerResolveRefsFailsWhenDefaultsMissing(t *testing.T) {
	repoRoot := initReviewRepo(t, "trunk")
	writeReviewFile(t, repoRoot, "app.txt", []byte("base\n"))
	commitReviewAll(t, repoRoot, "initial")

	reviewer := NewReviewer(llm.NewMockClient(), zap.NewNop(), testReviewConfig())
	_, _, err := reviewer.resolveRefs(context.Background(), repoRoot, Params{})
	if err == nil {
		t.Fatal("expected resolveRefs to fail when no default base refs exist")
	}
	text := err.Error()
	if !strings.Contains(text, "origin/HEAD, origin/main, main, origin/master, master") {
		t.Fatalf("expected tried refs in error, got %q", text)
	}
	if !strings.Contains(text, "Pass --base explicitly") {
		t.Fatalf("expected explicit --base guidance, got %q", text)
	}
}

func testReviewConfig() config.Config {
	return config.Config{
		Model:                  config.DefaultModel,
		ToolTimeoutSeconds:     10,
		ToolParallelism:        4,
		ToolLimits:             config.ToolLimits{GrepMaxResults: 20, GrepMaxBytes: 64 * 1024, ContextMaxBytes: 128 * 1024, MaxFileBytes: 64 * 1024},
		LLMRetryInitialBackoff: time.Millisecond,
		LLMRetryMaxBackoff:     time.Millisecond,
		ReviewMaxFiles:         config.DefaultReviewMaxFiles,
		ReviewMaxFindings:      config.DefaultReviewMaxFindings,
		ReviewInstructionsFile: ".fi/review.md",
		ReviewPathRulesFile:    ".fi/review-paths.yaml",
	}
}

func initReviewRepo(t *testing.T, branch string) string {
	t.Helper()
	dir := t.TempDir()
	runReviewGit(t, dir, "init")
	runReviewGit(t, dir, "checkout", "-b", branch)
	runReviewGit(t, dir, "config", "user.name", "Test User")
	runReviewGit(t, dir, "config", "user.email", "test@example.com")
	return dir
}

func commitReviewAll(t *testing.T, dir string, message string) {
	t.Helper()
	runReviewGit(t, dir, "add", "-A")
	runReviewGit(t, dir, "commit", "-m", message)
}

func writeReviewFile(t *testing.T, repoRoot string, relPath string, data []byte) {
	t.Helper()
	absPath := filepath.Join(repoRoot, relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
	if err := os.WriteFile(absPath, data, 0o644); err != nil {
		t.Fatalf("failed to write file %s: %v", relPath, err)
	}
}

func runReviewGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}
