package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"fi-cli/internal/config"
	"fi-cli/internal/llm"
	"fi-cli/internal/repo"
	"fi-cli/internal/tools"

	"go.uber.org/zap"
)

type delayedBenchmarkClient struct {
	createCalls int
	firstDelay  time.Duration
	nextDelay   time.Duration
}

func (c *delayedBenchmarkClient) Create(ctx context.Context, req llm.Request) (llm.Response, error) {
	c.createCalls++
	if c.createCalls == 1 {
		time.Sleep(c.firstDelay)
		args, _ := json.Marshal(map[string]any{"pattern": "FICLI"})
		return llm.Response{ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "grep", Arguments: args}}}, nil
	}
	time.Sleep(c.nextDelay)
	return llm.Response{Content: "done"}, nil
}

func (c *delayedBenchmarkClient) Stream(ctx context.Context, req llm.Request, onDelta func(string)) (llm.Response, error) {
	resp := llm.Response{Content: "done"}
	if onDelta != nil {
		onDelta(resp.Content)
	}
	return resp, nil
}

type delayedBenchmarkTool struct {
	delay time.Duration
}

func (t delayedBenchmarkTool) Name() string        { return "grep" }
func (t delayedBenchmarkTool) Description() string { return "delayed benchmark tool" }
func (t delayedBenchmarkTool) Schema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"pattern": map[string]any{"type": "string"}}}
}

func (t delayedBenchmarkTool) Execute(ctx context.Context, input json.RawMessage, meta tools.Meta) (tools.Result, error) {
	time.Sleep(t.delay)
	payload := map[string]any{"matches": []string{"file.txt:1:FICLI"}, "truncated": false}
	return tools.Result{ToolName: "grep", Payload: payload, Preview: "file.txt:1:FICLI", LineCount: 1, ByteCount: 18, Truncated: false}, nil
}

func BenchmarkAgentStageTimings(b *testing.B) {
	logger := zap.NewNop()
	cfg := config.Config{
		Model:      config.DefaultModel,
		MaxSteps:   4,
		JSON:       true,
		NoPlan:     true,
		NoHistory:  true,
		ToolLimits: config.ToolLimits{GrepMaxResults: 10, GrepMaxBytes: 1024, ShellMaxBytes: 1024, WebMaxBytes: 1024, ContextMaxBytes: 4096, MaxFileBytes: 1024, GrepMaxCalls: 5, ShellMaxCalls: 5, WebMaxCalls: 5},
	}

	const (
		firstModelDelay    = 45 * time.Millisecond
		followupModelDelay = 15 * time.Millisecond
		toolDelay          = 30 * time.Millisecond
		regressionBudget   = int64(1500)
	)

	var sumFirstModel int64
	var sumToolTotal int64
	var sumTotal int64

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client := &delayedBenchmarkClient{firstDelay: firstModelDelay, nextDelay: followupModelDelay}
		ag := NewAgent(client, tools.NewRegistry(delayedBenchmarkTool{delay: toolDelay}), nil, logger, cfg)
		result, err := ag.Run(context.Background(), "find signal", "/tmp", repo.RepoContext{RepoRoot: "/tmp"})
		if err != nil {
			b.Fatalf("unexpected run error: %v", err)
		}

		firstModelMs := result.StageTimingsMs[StageFirstModelResponseWait]
		toolTotalMs := result.StageTimingsMs[StageToolExecutionTotal]
		totalMs := result.StageTimingsMs[StageTotalRunDuration]

		if totalMs > regressionBudget {
			b.Fatalf("startup regression threshold exceeded: total_run_duration=%dms budget=%dms", totalMs, regressionBudget)
		}

		dominantStage := StageFirstModelResponseWait
		if toolTotalMs > firstModelMs {
			dominantStage = StageToolExecutionTotal
		}
		if dominantStage != StageFirstModelResponseWait && dominantStage != StageToolExecutionTotal {
			b.Fatalf("unexpected dominant stage: %s", dominantStage)
		}

		sumFirstModel += firstModelMs
		sumToolTotal += toolTotalMs
		sumTotal += totalMs
	}

	if b.N > 0 {
		b.ReportMetric(float64(sumFirstModel)/float64(b.N), "first_model_ms")
		b.ReportMetric(float64(sumToolTotal)/float64(b.N), "tool_total_ms")
		b.ReportMetric(float64(sumTotal)/float64(b.N), "total_run_ms")
	}
}
