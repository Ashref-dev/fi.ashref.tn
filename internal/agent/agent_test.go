package agent

import (
	"context"
	"encoding/json"
	"testing"

	"ag-cli/internal/config"
	"ag-cli/internal/llm"
	"ag-cli/internal/repo"
	"ag-cli/internal/tools"

	"go.uber.org/zap"
)

type fakeTool struct{}

func (f fakeTool) Name() string        { return "grep" }
func (f fakeTool) Description() string { return "fake tool" }
func (f fakeTool) Schema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"pattern": map[string]any{"type": "string"}}, "required": []string{"pattern"}}
}
func (f fakeTool) Execute(ctx context.Context, input json.RawMessage, meta tools.Meta) (tools.Result, error) {
	payload := map[string]any{"matches": []string{"file.txt:1:AGCLI"}, "truncated": false, "duration_ms": 1}
	return tools.Result{ToolName: "grep", Payload: payload, Preview: "file.txt:1:AGCLI", LineCount: 1, ByteCount: 18, Truncated: false, DurationMs: 1}, nil
}

func TestAgentRunWithMock(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := config.Config{Model: config.DefaultModel, MaxSteps: 4, JSON: true, NoHistory: true, ToolLimits: config.ToolLimits{GrepMaxResults: 10, GrepMaxBytes: 1024, ShellMaxBytes: 1024, WebMaxBytes: 1024, ContextMaxBytes: 4096, MaxFileBytes: 1024}}
	client := llm.NewMockClient()
	registry := tools.NewRegistry(fakeTool{})
	repoCtx := repo.RepoContext{RepoRoot: "/tmp"}
	ag := NewAgent(client, registry, nil, logger, cfg)

	result, err := ag.Run(context.Background(), "test question", "/tmp", repoCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.FinalAnswer == "" {
		t.Fatalf("expected final answer")
	}
	if len(result.ToolCalls) == 0 {
		t.Fatalf("expected tool calls")
	}
}
