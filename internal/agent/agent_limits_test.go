package agent

import (
	"context"
	"encoding/json"
	"testing"

	"fi-cli/internal/config"
	"fi-cli/internal/llm"
	"fi-cli/internal/repo"
	"fi-cli/internal/tools"

	"go.uber.org/zap"
)

type sequenceClient struct {
	responses []llm.Response
	index     int
}

func (c *sequenceClient) Create(ctx context.Context, req llm.Request) (llm.Response, error) {
	if c.index >= len(c.responses) {
		return llm.Response{Content: "done"}, nil
	}
	resp := c.responses[c.index]
	c.index++
	return resp, nil
}

func (c *sequenceClient) Stream(ctx context.Context, req llm.Request, onDelta func(string)) (llm.Response, error) {
	resp := llm.Response{Content: "done"}
	if onDelta != nil {
		onDelta(resp.Content)
	}
	return resp, nil
}

func TestAgentToolCallBudget(t *testing.T) {
	logger := zap.NewNop()
	args, _ := json.Marshal(map[string]any{"pattern": "abc"})
	client := &sequenceClient{
		responses: []llm.Response{
			{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "grep", Arguments: args}}},
			{ToolCalls: []llm.ToolCall{{ID: "c2", Name: "grep", Arguments: args}}},
			{Content: "final"},
		},
	}
	cfg := config.Config{
		Model:      config.DefaultModel,
		MaxSteps:   5,
		JSON:       true,
		NoPlan:     true,
		NoHistory:  true,
		ToolLimits: config.ToolLimits{GrepMaxResults: 10, GrepMaxBytes: 1024, ShellMaxBytes: 1024, WebMaxBytes: 1024, ContextMaxBytes: 4096, MaxFileBytes: 1024, GrepMaxCalls: 1, ShellMaxCalls: 1, WebMaxCalls: 1},
	}
	ag := NewAgent(client, tools.NewRegistry(fakeTool{}), nil, logger, cfg)
	result, err := ag.Run(context.Background(), "find pattern", "/tmp", repo.RepoContext{RepoRoot: "/tmp"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "success" {
		t.Fatalf("expected success status, got %s", result.Status)
	}
	if len(result.ToolCalls) != 2 {
		t.Fatalf("expected 2 tool call records, got %d", len(result.ToolCalls))
	}
	if result.ToolCalls[0].Status != "success" {
		t.Fatalf("first call should succeed")
	}
	if result.ToolCalls[1].Status != "error" {
		t.Fatalf("second call should fail with budget error")
	}
}
