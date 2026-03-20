package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitRefsAndMergeBaseTools(t *testing.T) {
	repoRoot := initGitRepo(t, "main")
	writeGitFile(t, repoRoot, "app.txt", []byte("one\n"))
	commitAll(t, repoRoot, "initial")
	mainCommit := runGit(t, repoRoot, "rev-parse", "HEAD")

	runGit(t, repoRoot, "checkout", "-b", "feature")
	writeGitFile(t, repoRoot, "app.txt", []byte("one\ntwo\n"))
	commitAll(t, repoRoot, "feature change")

	refsOutput := executeGitTool[GitRefsOutput](t, NewGitRefsTool(), repoRoot, map[string]any{
		"candidates": []string{"origin/main", "main"},
	})
	if refsOutput.HeadRef != "feature" {
		t.Fatalf("expected head ref feature, got %q", refsOutput.HeadRef)
	}
	if len(refsOutput.Candidates) != 2 {
		t.Fatalf("expected 2 ref candidates, got %d", len(refsOutput.Candidates))
	}
	if refsOutput.Candidates[0].Exists {
		t.Fatalf("expected origin/main to be missing in local-only repo")
	}
	if !refsOutput.Candidates[1].Exists {
		t.Fatalf("expected main candidate to exist")
	}

	mergeBase := executeGitTool[GitMergeBaseOutput](t, NewGitMergeBaseTool(), repoRoot, map[string]any{
		"base_ref": "main",
		"head_ref": "HEAD",
	})
	if mergeBase.MergeBase != mainCommit {
		t.Fatalf("expected merge base %q, got %q", mainCommit, mergeBase.MergeBase)
	}
}

func TestGitChangedFilesToolDetectsStatuses(t *testing.T) {
	repoRoot := initGitRepo(t, "main")
	writeGitFile(t, repoRoot, "keep.txt", []byte("alpha\n"))
	writeGitFile(t, repoRoot, "rename.txt", []byte("rename me\n"))
	writeGitFile(t, repoRoot, "delete.txt", []byte("delete me\n"))
	writeGitFile(t, repoRoot, "image.bin", []byte{0x00, 0x01, 0x02})
	commitAll(t, repoRoot, "initial")

	runGit(t, repoRoot, "checkout", "-b", "feature")
	writeGitFile(t, repoRoot, "keep.txt", []byte("alpha\nbeta\n"))
	runGit(t, repoRoot, "mv", "rename.txt", "renamed.txt")
	writeGitFile(t, repoRoot, "renamed.txt", []byte("rename me\nupdated\n"))
	if err := os.Remove(filepath.Join(repoRoot, "delete.txt")); err != nil {
		t.Fatalf("failed to remove delete.txt: %v", err)
	}
	writeGitFile(t, repoRoot, "new.txt", []byte("new file\n"))
	writeGitFile(t, repoRoot, "image.bin", []byte{0x00, 0x03, 0x04, 0x05})
	commitAll(t, repoRoot, "feature change")

	output := executeGitTool[GitChangedFilesOutput](t, NewGitChangedFilesTool(), repoRoot, map[string]any{
		"base_ref": "main",
		"head_ref": "HEAD",
	})
	if len(output.ChangedFiles) != 5 {
		t.Fatalf("expected 5 changed files, got %d: %+v", len(output.ChangedFiles), output.ChangedFiles)
	}

	filesByPath := map[string]GitChangedFile{}
	for _, file := range output.ChangedFiles {
		filesByPath[file.Path] = file
	}

	if got := filesByPath["keep.txt"]; got.Status != "M" || got.Additions == 0 {
		t.Fatalf("expected keep.txt to be modified with additions, got %+v", got)
	}
	if got := filesByPath["new.txt"]; got.Status != "A" {
		t.Fatalf("expected new.txt added, got %+v", got)
	}
	if got := filesByPath["delete.txt"]; got.Status != "D" {
		t.Fatalf("expected delete.txt deleted, got %+v", got)
	}
	if got := filesByPath["renamed.txt"]; got.Status != "R" || got.OldPath != "rename.txt" {
		t.Fatalf("expected renamed.txt rename from rename.txt, got %+v", got)
	}
	if got := filesByPath["image.bin"]; got.Status != "M" || !got.Binary {
		t.Fatalf("expected image.bin binary modification, got %+v", got)
	}
}

func TestGitDiffHunksToolParsesAddedAndDeletedLines(t *testing.T) {
	repoRoot := initGitRepo(t, "main")
	writeGitFile(t, repoRoot, "service.txt", []byte("one\ntwo\nthree\n"))
	commitAll(t, repoRoot, "initial")

	runGit(t, repoRoot, "checkout", "-b", "feature")
	writeGitFile(t, repoRoot, "service.txt", []byte("one\nTWO\nthree\nfour\n"))
	commitAll(t, repoRoot, "feature change")

	output := executeGitTool[GitDiffHunksOutput](t, NewGitDiffHunksTool(), repoRoot, map[string]any{
		"base_ref": "main",
		"head_ref": "HEAD",
		"path":     "service.txt",
	})
	if !strings.Contains(output.Patch, "@@") {
		t.Fatalf("expected unified diff patch, got %q", output.Patch)
	}
	if len(output.Hunks) == 0 {
		t.Fatalf("expected parsed diff hunks")
	}
	first := output.Hunks[0]
	if !containsInt(first.AddedLines, 2) || !containsInt(first.AddedLines, 4) {
		t.Fatalf("expected added lines to include 2 and 4, got %+v", first.AddedLines)
	}
	if !containsInt(first.DeletedLines, 2) {
		t.Fatalf("expected deleted lines to include 2, got %+v", first.DeletedLines)
	}
}

func executeGitTool[T any](t *testing.T, tool Tool, repoRoot string, input any) T {
	t.Helper()
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("failed to marshal tool input: %v", err)
	}
	result, err := tool.Execute(context.Background(), data, Meta{
		RepoRoot:           repoRoot,
		ToolTimeoutSeconds: 10,
		MaxBytes:           512 * 1024,
		MaxResults:         200,
	})
	if err != nil {
		t.Fatalf("tool %s failed: %v", tool.Name(), err)
	}
	payload, ok := result.Payload.(T)
	if !ok {
		t.Fatalf("unexpected payload type %T", result.Payload)
	}
	return payload
}

func initGitRepo(t *testing.T, branch string) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "checkout", "-b", branch)
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	return dir
}

func commitAll(t *testing.T, dir string, message string) {
	t.Helper()
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", message)
}

func writeGitFile(t *testing.T, repoRoot string, relPath string, data []byte) {
	t.Helper()
	absPath := filepath.Join(repoRoot, relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
	if err := os.WriteFile(absPath, data, 0o644); err != nil {
		t.Fatalf("failed to write file %s: %v", relPath, err)
	}
}

func runGit(t *testing.T, dir string, args ...string) string {
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

func containsInt(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
