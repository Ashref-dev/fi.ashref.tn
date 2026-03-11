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
	stageTimings, ok := payload["stage_timings_ms"].(map[string]any)
	if !ok {
		t.Fatalf("expected stage_timings_ms object, got %T", payload["stage_timings_ms"])
	}
	required := []string{
		"question_resolution_input",
		"config_load",
		"repo_root_resolution",
		"repo_context_build",
		"history_load",
		"planning",
		"first_model_response_wait",
		"tool_execution_total",
		"first_answer_token_latency",
		"total_run_duration",
	}
	for _, key := range required {
		raw, exists := stageTimings[key]
		if !exists {
			t.Fatalf("missing stage timing key %q", key)
		}
		value, ok := raw.(float64)
		if !ok {
			t.Fatalf("expected numeric timing for %q, got %T", key, raw)
		}
		if value < 0 {
			t.Fatalf("expected non-negative timing for %q, got %f", key, value)
		}
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

func TestCLITimingsOutputAndPlanBehavior(t *testing.T) {
	fixture := t.TempDir()
	if err := os.WriteFile(filepath.Join(fixture, "sample.txt"), []byte("FICLI test\n"), 0o644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}
	wd, _ := os.Getwd()
	root := filepath.Dir(filepath.Dir(wd))

	noPlanCmd := exec.Command("go", "run", "./cmd/fi-cli", "--timings", "--repo", fixture, "test question")
	noPlanCmd.Env = append(os.Environ(), "FICLI_MOCK_LLM=1")
	noPlanCmd.Dir = root

	noPlanOut, err := noPlanCmd.Output()
	if err != nil {
		t.Fatalf("command failed: %v", err)
	}
	text := string(noPlanOut)
	if strings.Contains(text, "\nPlan:\n") {
		t.Fatalf("expected default no-plan behavior, got output: %s", text)
	}
	for _, key := range []string{
		"question_resolution_input:",
		"config_load:",
		"repo_root_resolution:",
		"repo_context_build:",
		"history_load:",
		"planning:",
		"first_model_response_wait:",
		"tool_execution_total:",
		"first_answer_token_latency:",
		"total_run_duration:",
		"slowest_stage:",
	} {
		if !strings.Contains(text, key) {
			t.Fatalf("expected timings output to contain %q, got %s", key, text)
		}
	}

	planCmd := exec.Command("go", "run", "./cmd/fi-cli", "--plan", "--repo", fixture, "test question")
	planCmd.Env = append(os.Environ(), "FICLI_MOCK_LLM=1")
	planCmd.Dir = root

	planOut, err := planCmd.Output()
	if err != nil {
		t.Fatalf("command failed: %v", err)
	}
	if !strings.Contains(string(planOut), "\nPlan:\n") {
		t.Fatalf("expected --plan output to include plan, got %s", string(planOut))
	}
}

func TestCLIDefaultNoPlanWithToolExecutionOutput(t *testing.T) {
	fixture := t.TempDir()
	if err := os.WriteFile(filepath.Join(fixture, "sample.txt"), []byte("FICLI test\n"), 0o644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}
	wd, _ := os.Getwd()
	root := filepath.Dir(filepath.Dir(wd))

	cmd := exec.Command("go", "run", "./cmd/fi-cli", "--show-tools", "--repo", fixture, "test question")
	cmd.Env = append(os.Environ(), "FICLI_MOCK_LLM=1")
	cmd.Dir = root

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("command failed: %v", err)
	}
	text := string(out)
	if strings.Contains(text, "\nPlan:\n") {
		t.Fatalf("expected default no-plan behavior, got output: %s", text)
	}
	if !strings.Contains(text, "tool: grep ok") {
		t.Fatalf("expected tool execution line, got output: %s", text)
	}
	if !strings.Contains(text, "fi: ") {
		t.Fatalf("expected final answer prefix, got output: %s", text)
	}
}
