package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"fi-cli/internal/config"
	"fi-cli/internal/llm"
	"fi-cli/internal/repo"
	"fi-cli/internal/tools"

	"go.uber.org/zap"
)

type recordedSequenceClient struct {
	responses []llm.Response
	requests  []llm.Request
	index     int
}

func (c *recordedSequenceClient) Create(ctx context.Context, req llm.Request) (llm.Response, error) {
	c.requests = append(c.requests, req)
	if c.index >= len(c.responses) {
		return llm.Response{Content: "done"}, nil
	}
	resp := c.responses[c.index]
	c.index++
	return resp, nil
}

func (c *recordedSequenceClient) Stream(ctx context.Context, req llm.Request, onDelta func(string)) (llm.Response, error) {
	c.requests = append(c.requests, req)
	resp := llm.Response{Content: "done"}
	if onDelta != nil {
		onDelta(resp.Content)
	}
	return resp, nil
}

type controlledTool struct {
	name string
}

func (t controlledTool) Name() string        { return t.name }
func (t controlledTool) Description() string { return "controlled tool" }
func (t controlledTool) Schema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"sleep_ms": map[string]any{"type": "integer"}, "fail": map[string]any{"type": "boolean"}}}
}

func (t controlledTool) Execute(ctx context.Context, input json.RawMessage, meta tools.Meta) (tools.Result, error) {
	if meta.ToolTimeoutSeconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(meta.ToolTimeoutSeconds)*time.Second)
		defer cancel()
	}
	var args struct {
		SleepMs int  `json:"sleep_ms"`
		Fail    bool `json:"fail"`
	}
	_ = json.Unmarshal(input, &args)
	if args.SleepMs > 0 {
		select {
		case <-ctx.Done():
			return tools.Result{}, ctx.Err()
		case <-time.After(time.Duration(args.SleepMs) * time.Millisecond):
		}
	}
	if args.Fail {
		return tools.Result{}, errors.New("forced tool failure")
	}
	payload := map[string]any{"tool": t.name, "ok": true}
	return tools.Result{ToolName: t.name, Payload: payload, Preview: t.name, LineCount: 1, ByteCount: len(t.name), Truncated: false}, nil
}

func toolArgs(sleepMs int, fail bool) json.RawMessage {
	args, _ := json.Marshal(map[string]any{"sleep_ms": sleepMs, "fail": fail})
	return args
}

func baseAgentConfig() config.Config {
	return config.Config{
		Model:                  config.DefaultModel,
		MaxSteps:               4,
		JSON:                   true,
		NoPlan:                 true,
		NoHistory:              true,
		ToolTimeoutSeconds:     5,
		ToolParallelism:        4,
		ToolLimits:             config.ToolLimits{GrepMaxResults: 50, GrepMaxBytes: 32 * 1024, ShellMaxBytes: 32 * 1024, WebMaxBytes: 32 * 1024, ContextMaxBytes: 32 * 1024, MaxFileBytes: 32 * 1024, GrepMaxCalls: 30, ShellMaxCalls: 30, WebMaxCalls: 30},
		LLMRetryMaxAttempts:    0,
		LLMRetryInitialBackoff: 1 * time.Millisecond,
		LLMRetryMaxBackoff:     1 * time.Millisecond,
	}
}

func TestParallelReadOnlyToolBatchIsFasterThanSequentialFallback(t *testing.T) {
	logger := zap.NewNop()
	cfg := baseAgentConfig()

	parallelClient := &recordedSequenceClient{responses: []llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "p1", Name: "grep", Arguments: toolArgs(180, false)}, {ID: "p2", Name: "exa_search", Arguments: toolArgs(180, false)}}},
		{Content: "done"},
	}}
	parallelAgent := NewAgent(parallelClient, tools.NewRegistry(controlledTool{name: "grep"}, controlledTool{name: "exa_search"}), nil, logger, cfg)
	start := time.Now()
	if _, err := parallelAgent.Run(context.Background(), "parallel", "/tmp", repo.RepoContext{RepoRoot: "/tmp"}); err != nil {
		t.Fatalf("parallel run failed: %v", err)
	}
	parallelElapsed := time.Since(start)

	sequentialClient := &recordedSequenceClient{responses: []llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "s1", Name: "grep", Arguments: toolArgs(180, false)}, {ID: "s2", Name: "shell", Arguments: toolArgs(180, false)}}},
		{Content: "done"},
	}}
	sequentialAgent := NewAgent(sequentialClient, tools.NewRegistry(controlledTool{name: "grep"}, controlledTool{name: "shell"}), nil, logger, cfg)
	start = time.Now()
	if _, err := sequentialAgent.Run(context.Background(), "sequential", "/tmp", repo.RepoContext{RepoRoot: "/tmp"}); err != nil {
		t.Fatalf("sequential run failed: %v", err)
	}
	sequentialElapsed := time.Since(start)

	if !(parallelElapsed < 300*time.Millisecond) {
		t.Fatalf("expected parallel batch to be fast, got %s", parallelElapsed)
	}
	if !(sequentialElapsed > 320*time.Millisecond) {
		t.Fatalf("expected sequential fallback to be slower, got %s", sequentialElapsed)
	}
}

func TestParallelBatchContinuesWhenOneToolFails(t *testing.T) {
	logger := zap.NewNop()
	cfg := baseAgentConfig()
	client := &recordedSequenceClient{responses: []llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "grep", Arguments: toolArgs(20, true)}, {ID: "c2", Name: "exa_search", Arguments: toolArgs(120, false)}}},
		{Content: "done"},
	}}
	ag := NewAgent(client, tools.NewRegistry(controlledTool{name: "grep"}, controlledTool{name: "exa_search"}), nil, logger, cfg)
	result, err := ag.Run(context.Background(), "continue", "/tmp", repo.RepoContext{RepoRoot: "/tmp"})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(result.ToolCalls) != 2 {
		t.Fatalf("expected 2 tool call records, got %d", len(result.ToolCalls))
	}
	if result.ToolCalls[0].Status != "error" {
		t.Fatalf("expected first call error, got %s", result.ToolCalls[0].Status)
	}
	if result.ToolCalls[1].Status != "success" {
		t.Fatalf("expected second call success, got %s", result.ToolCalls[1].Status)
	}
}

func TestParallelBatchTimeoutDoesNotCancelSiblingCalls(t *testing.T) {
	logger := zap.NewNop()
	cfg := baseAgentConfig()
	cfg.ToolTimeoutSeconds = 1
	client := &recordedSequenceClient{responses: []llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "t1", Name: "grep", Arguments: toolArgs(1500, false)}, {ID: "t2", Name: "exa_search", Arguments: toolArgs(200, false)}}},
		{Content: "done"},
	}}
	ag := NewAgent(client, tools.NewRegistry(controlledTool{name: "grep"}, controlledTool{name: "exa_search"}), nil, logger, cfg)
	result, err := ag.Run(context.Background(), "timeout", "/tmp", repo.RepoContext{RepoRoot: "/tmp"})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(result.ToolCalls) != 2 {
		t.Fatalf("expected 2 tool call records, got %d", len(result.ToolCalls))
	}
	if result.ToolCalls[0].Status != "error" {
		t.Fatalf("expected first call timeout error, got %s", result.ToolCalls[0].Status)
	}
	if result.ToolCalls[1].Status != "success" {
		t.Fatalf("expected sibling call success, got %s", result.ToolCalls[1].Status)
	}
}

func TestParallelBatchKeepsToolMessageOrderStable(t *testing.T) {
	logger := zap.NewNop()
	cfg := baseAgentConfig()
	client := &recordedSequenceClient{responses: []llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "a", Name: "grep", Arguments: toolArgs(160, false)}, {ID: "b", Name: "exa_search", Arguments: toolArgs(20, false)}}},
		{Content: "done"},
	}}
	ag := NewAgent(client, tools.NewRegistry(controlledTool{name: "grep"}, controlledTool{name: "exa_search"}), nil, logger, cfg)
	_, err := ag.Run(context.Background(), "ordered", "/tmp", repo.RepoContext{RepoRoot: "/tmp"})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(client.requests) < 2 {
		t.Fatalf("expected at least two model requests")
	}
	payload, _ := json.Marshal(client.requests[1].Messages)
	text := string(payload)
	first := strings.Index(text, `"tool_call_id":"a"`)
	second := strings.Index(text, `"tool_call_id":"b"`)
	if first == -1 || second == -1 {
		t.Fatalf("expected tool messages with IDs a and b, got %s", text)
	}
	if first > second {
		t.Fatalf("expected tool message order a before b, got %s", text)
	}
}
