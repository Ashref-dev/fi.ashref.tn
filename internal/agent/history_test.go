package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fi-cli/internal/config"
	"fi-cli/internal/llm"
	"fi-cli/internal/repo"
	"fi-cli/internal/tools"

	"go.uber.org/zap"
)

type captureClient struct {
	requests []llm.Request
}

func (c *captureClient) Create(ctx context.Context, req llm.Request) (llm.Response, error) {
	c.requests = append(c.requests, req)
	return llm.Response{Content: "ok"}, nil
}

func (c *captureClient) Stream(ctx context.Context, req llm.Request, onDelta func(string)) (llm.Response, error) {
	c.requests = append(c.requests, req)
	if onDelta != nil {
		onDelta("ok")
	}
	return llm.Response{Content: "ok"}, nil
}

func TestAgentIncludesShellHistoryWhenEnabled(t *testing.T) {
	historyDir := t.TempDir()
	historyPath := filepath.Join(historyDir, ".zsh_history")
	history := strings.Join([]string{
		": 1710000000:0;git status",
		": 1710000001:0;ls -la",
	}, "\n") + "\n"
	if err := os.WriteFile(historyPath, []byte(history), 0o644); err != nil {
		t.Fatalf("failed to write history: %v", err)
	}
	t.Setenv("HISTFILE", historyPath)

	client := &captureClient{}
	cfg := config.Config{
		Model:        config.DefaultModel,
		MaxSteps:     2,
		JSON:         true,
		NoPlan:       true,
		NoHistory:    false,
		HistoryLines: 5,
		ToolLimits:   config.ToolLimits{GrepMaxResults: 10, GrepMaxBytes: 1024, ShellMaxBytes: 1024, WebMaxBytes: 1024, ContextMaxBytes: 4096, MaxFileBytes: 1024, GrepMaxCalls: 5, ShellMaxCalls: 5, WebMaxCalls: 5},
	}
	logger := zap.NewNop()
	ag := NewAgent(client, tools.NewRegistry(), nil, logger, cfg)

	_, err := ag.Run(context.Background(), "hello", "/tmp", repo.RepoContext{RepoRoot: "/tmp"})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(client.requests) == 0 {
		t.Fatalf("expected model requests")
	}

	payload, err := json.Marshal(client.requests[0].Messages)
	if err != nil {
		t.Fatalf("failed to marshal messages: %v", err)
	}
	text := string(payload)
	if !strings.Contains(text, "Recent shell history (most recent last)") {
		t.Fatalf("expected history block in messages, got %s", text)
	}
	if !strings.Contains(text, "git status") || !strings.Contains(text, "ls -la") {
		t.Fatalf("expected history lines in messages, got %s", text)
	}
}

func TestAgentSkipsShellHistoryWhenDisabled(t *testing.T) {
	historyDir := t.TempDir()
	historyPath := filepath.Join(historyDir, ".zsh_history")
	if err := os.WriteFile(historyPath, []byte(": 1710000000:0;git status\n"), 0o644); err != nil {
		t.Fatalf("failed to write history: %v", err)
	}
	t.Setenv("HISTFILE", historyPath)

	client := &captureClient{}
	cfg := config.Config{
		Model:        config.DefaultModel,
		MaxSteps:     2,
		JSON:         true,
		NoPlan:       true,
		NoHistory:    true,
		HistoryLines: 5,
		ToolLimits:   config.ToolLimits{GrepMaxResults: 10, GrepMaxBytes: 1024, ShellMaxBytes: 1024, WebMaxBytes: 1024, ContextMaxBytes: 4096, MaxFileBytes: 1024, GrepMaxCalls: 5, ShellMaxCalls: 5, WebMaxCalls: 5},
	}
	logger := zap.NewNop()
	ag := NewAgent(client, tools.NewRegistry(), nil, logger, cfg)

	_, err := ag.Run(context.Background(), "hello", "/tmp", repo.RepoContext{RepoRoot: "/tmp"})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(client.requests) == 0 {
		t.Fatalf("expected model requests")
	}

	payload, err := json.Marshal(client.requests[0].Messages)
	if err != nil {
		t.Fatalf("failed to marshal messages: %v", err)
	}
	if strings.Contains(string(payload), "Recent shell history (most recent last)") {
		t.Fatalf("did not expect history block when disabled")
	}
}
