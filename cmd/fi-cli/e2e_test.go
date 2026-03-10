package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIJSONOutput(t *testing.T) {
	fixture := t.TempDir()
	if err := os.WriteFile(filepath.Join(fixture, "sample.txt"), []byte("FICLI test\n"), 0o644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	cmd := exec.Command("go", "run", "./cmd/fi-cli", "--json", "--repo", fixture, "test question")
	cmd.Env = append(os.Environ(), "FICLI_MOCK_LLM=1")
	wd, _ := os.Getwd()
	cmd.Dir = filepath.Dir(filepath.Dir(wd))

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("command failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("invalid json output: %v", err)
	}
	if payload["run_id"] == "" {
		t.Fatalf("expected run_id")
	}
	if payload["final_answer"] == "" {
		t.Fatalf("expected final_answer")
	}
}

func TestCLIJSONOutputFromStdinQuestion(t *testing.T) {
	fixture := t.TempDir()
	if err := os.WriteFile(filepath.Join(fixture, "sample.txt"), []byte("FICLI test\n"), 0o644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	cmd := exec.Command("go", "run", "./cmd/fi-cli", "--json", "--repo", fixture)
	cmd.Env = append(os.Environ(), "FICLI_MOCK_LLM=1")
	cmd.Stdin = strings.NewReader("test question from stdin\n")
	wd, _ := os.Getwd()
	cmd.Dir = filepath.Dir(filepath.Dir(wd))

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("command failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("invalid json output: %v", err)
	}
	if payload["question"] != "test question from stdin" {
		t.Fatalf("expected stdin question to be used, got %v", payload["question"])
	}
}
