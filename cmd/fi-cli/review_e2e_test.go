package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var (
	reviewBinaryOnce sync.Once
	reviewBinaryPath string
	reviewBinaryErr  error
)

func TestCLIReviewJSONOutputNoBlockers(t *testing.T) {
	repoRoot := initCLIGitRepo(t)
	writeCLIRepoFile(t, repoRoot, "app.txt", []byte("base\n"))
	commitCLIRepo(t, repoRoot, "initial")
	runCLIGit(t, repoRoot, "checkout", "-b", "feature")
	writeCLIRepoFile(t, repoRoot, "app.txt", []byte("base\nchange\n"))
	commitCLIRepo(t, repoRoot, "feature change")

	out, err := runReviewCLI(t, repoRoot, "--json", "--base", "main")
	if err != nil {
		t.Fatalf("review command failed: %v\n%s", err, out)
	}
	payload := decodeReviewJSON(t, out)
	for _, key := range []string{"base_ref", "head_ref", "merge_base", "score_5", "merge_ready", "blocker_count", "summary", "strengths", "coverage", "skipped_files", "reviewed_files", "findings", "stage_timings_ms"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("expected JSON payload key %q, got %v", key, payload)
		}
	}
	if payload["merge_ready"] != true {
		t.Fatalf("expected merge_ready true, got %v", payload["merge_ready"])
	}
	if payload["blocker_count"].(float64) != 0 {
		t.Fatalf("expected blocker_count 0, got %v", payload["blocker_count"])
	}
	if len(payload["reviewed_files"].([]any)) == 0 {
		t.Fatalf("expected reviewed_files to be non-empty, got %v", payload["reviewed_files"])
	}
}

func TestCLIReviewBlockersExitCodeThree(t *testing.T) {
	repoRoot := initCLIGitRepo(t)
	writeCLIRepoFile(t, repoRoot, "app.txt", []byte("base\n"))
	commitCLIRepo(t, repoRoot, "initial")
	runCLIGit(t, repoRoot, "checkout", "-b", "feature")
	writeCLIRepoFile(t, repoRoot, "app.txt", []byte("base\nreview:blocker\n"))
	commitCLIRepo(t, repoRoot, "introduce blocker")

	out, err := runReviewCLI(t, repoRoot, "--json", "--base", "main")
	if err == nil {
		t.Fatalf("expected blocker review to return exit code 3")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected exit error, got %T", err)
	}
	if exitErr.ExitCode() != 3 {
		t.Fatalf("expected exit code 3, got %d\n%s", exitErr.ExitCode(), out)
	}
	payload := decodeReviewJSON(t, out)
	if payload["merge_ready"] != false {
		t.Fatalf("expected merge_ready false, got %v", payload["merge_ready"])
	}
	if payload["blocker_count"].(float64) < 1 {
		t.Fatalf("expected blocker_count >= 1, got %v", payload["blocker_count"])
	}
}

func TestCLIReviewWorkingTreeOptIn(t *testing.T) {
	repoRoot := initCLIGitRepo(t)
	writeCLIRepoFile(t, repoRoot, "app.txt", []byte("base\n"))
	commitCLIRepo(t, repoRoot, "initial")
	writeCLIRepoFile(t, repoRoot, "app.txt", []byte("base\nreview:blocker\n"))

	out, err := runReviewCLI(t, repoRoot, "--json", "--base", "main")
	if err != nil {
		t.Fatalf("review without working-tree should succeed: %v\n%s", err, out)
	}
	payload := decodeReviewJSON(t, out)
	if payload["coverage"].(map[string]any)["working_tree_included"] != false {
		t.Fatalf("expected working_tree_included false, got %v", payload["coverage"])
	}
	if payload["coverage"].(map[string]any)["total_changed"].(float64) != 0 {
		t.Fatalf("expected total_changed 0 without --working-tree, got %v", payload["coverage"])
	}

	out, err = runReviewCLI(t, repoRoot, "--json", "--base", "main", "--working-tree")
	if err == nil {
		t.Fatalf("expected working-tree review to return blocker exit code 3")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected exit error, got %T", err)
	}
	if exitErr.ExitCode() != 3 {
		t.Fatalf("expected exit code 3, got %d\n%s", exitErr.ExitCode(), out)
	}
	payload = decodeReviewJSON(t, out)
	coverage := payload["coverage"].(map[string]any)
	if coverage["working_tree_included"] != true {
		t.Fatalf("expected working_tree_included true, got %v", coverage)
	}
	if coverage["total_changed"].(float64) < 1 {
		t.Fatalf("expected total_changed >= 1 with --working-tree, got %v", coverage)
	}
}

func TestCLIReviewTimingsOutput(t *testing.T) {
	repoRoot := initCLIGitRepo(t)
	writeCLIRepoFile(t, repoRoot, "app.txt", []byte("base\n"))
	commitCLIRepo(t, repoRoot, "initial")
	runCLIGit(t, repoRoot, "checkout", "-b", "feature")
	writeCLIRepoFile(t, repoRoot, "app.txt", []byte("base\nchange\n"))
	commitCLIRepo(t, repoRoot, "feature change")

	out, err := runReviewCLI(t, repoRoot, "--timings", "--base", "main")
	if err != nil {
		t.Fatalf("review timings command failed: %v\n%s", err, out)
	}
	text := string(out)
	for _, key := range []string{
		"review: base=main head=HEAD",
		"ref_resolution:",
		"merge_base_resolution:",
		"changed_files_scan:",
		"diff_context_build:",
		"triage:",
		"deep_review:",
		"model_wait_total:",
		"aggregation:",
		"total_run_duration:",
		"slowest_stage:",
	} {
		if !strings.Contains(text, key) {
			t.Fatalf("expected timings output to contain %q, got %s", key, text)
		}
	}
}

func runReviewCLI(t *testing.T, repoRoot string, args ...string) ([]byte, error) {
	t.Helper()
	fullArgs := append([]string{"review", "--repo", repoRoot}, args...)
	cmd := exec.Command(reviewBinary(t), fullArgs...)
	cmd.Env = append(os.Environ(), "FICLI_MOCK_LLM=1")
	return cmd.CombinedOutput()
}

func reviewBinary(t *testing.T) string {
	t.Helper()
	reviewBinaryOnce.Do(func() {
		dir, err := os.MkdirTemp("", "fi-cli-review-bin-*")
		if err != nil {
			reviewBinaryErr = err
			return
		}
		reviewBinaryPath = filepath.Join(dir, "fi-cli")
		wd, _ := os.Getwd()
		projectRoot := filepath.Dir(filepath.Dir(wd))
		cmd := exec.Command("go", "build", "-o", reviewBinaryPath, "./cmd/fi-cli")
		cmd.Dir = projectRoot
		output, err := cmd.CombinedOutput()
		if err != nil {
			reviewBinaryErr = errors.New(string(output))
		}
	})
	if reviewBinaryErr != nil {
		t.Fatalf("failed to build fi-cli test binary: %v", reviewBinaryErr)
	}
	return reviewBinaryPath
}

func decodeReviewJSON(t *testing.T, output []byte) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(output, &payload); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, output)
	}
	return payload
}

func initCLIGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runCLIGit(t, dir, "init")
	runCLIGit(t, dir, "checkout", "-b", "main")
	runCLIGit(t, dir, "config", "user.name", "Test User")
	runCLIGit(t, dir, "config", "user.email", "test@example.com")
	return dir
}

func commitCLIRepo(t *testing.T, dir string, message string) {
	t.Helper()
	runCLIGit(t, dir, "add", "-A")
	runCLIGit(t, dir, "commit", "-m", message)
}

func writeCLIRepoFile(t *testing.T, repoRoot string, relPath string, data []byte) {
	t.Helper()
	absPath := filepath.Join(repoRoot, relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
	if err := os.WriteFile(absPath, data, 0o644); err != nil {
		t.Fatalf("failed to write file %s: %v", relPath, err)
	}
}

func runCLIGit(t *testing.T, dir string, args ...string) string {
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
