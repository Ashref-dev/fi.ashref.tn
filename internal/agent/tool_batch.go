package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"fi-cli/internal/events"
	"fi-cli/internal/llm"
	"fi-cli/internal/tools"

	"github.com/openai/openai-go/v3"
)

type toolBatchOutcome struct {
	record      *ToolCallRecord
	toolMessage string
	executed    bool
	toolName    string
	durationMs  int64
}

type executableToolCall struct {
	index          int
	call           llm.ToolCall
	tool           tools.Tool
	inputSanitized any
	startedAt      time.Time
}

func (a *Agent) executeToolBatch(
	ctx context.Context,
	calls []llm.ToolCall,
	repoRoot string,
	usage map[string]int,
	result *RunResult,
	emit func(events.Event),
) []openai.ChatCompletionMessageParamUnion {
	batchStarted := time.Now()
	outcomes := make([]toolBatchOutcome, len(calls))
	executable := make([]executableToolCall, 0, len(calls))

	for i, call := range calls {
		inputSanitized := sanitizeInput(call.Arguments)
		if !a.withinToolBudget(call.Name, usage) {
			err := fmt.Errorf("tool call limit reached for %s", call.Name)
			payload := map[string]any{"error": err.Error(), "duration_ms": 0}
			record := ToolCallRecord{ToolName: call.Name, Input: inputSanitized, Output: payload, Status: "error", StartedAt: time.Now(), DurationMs: 0}
			emit(events.Event{Type: events.ToolCallFailed, Timestamp: time.Now(), Payload: events.ToolCallFinishedPayload{ToolName: call.Name, Status: "error", Preview: err.Error(), DurationMs: 0, LineCount: 1, ByteCount: len(err.Error()), Truncated: false}})
			payloadBytes, _ := json.Marshal(payload)
			outcomes[i] = toolBatchOutcome{record: &record, toolMessage: string(payloadBytes), toolName: call.Name}
			continue
		}
		tool, ok := a.tools.Get(call.Name)
		if !ok {
			err := fmt.Errorf("unknown tool: %s", call.Name)
			emit(events.Event{Type: events.ToolCallFailed, Timestamp: time.Now(), Payload: events.ToolCallFinishedPayload{ToolName: call.Name, Status: "error", Preview: err.Error(), DurationMs: 0, LineCount: 1, ByteCount: len(err.Error())}})
			payload := map[string]string{"error": err.Error()}
			payloadBytes, _ := json.Marshal(payload)
			record := ToolCallRecord{ToolName: call.Name, Input: inputSanitized, Output: payload, Status: "error", StartedAt: time.Now(), DurationMs: 0}
			outcomes[i] = toolBatchOutcome{record: &record, toolMessage: string(payloadBytes), toolName: call.Name}
			continue
		}

		usage[call.Name]++
		start := time.Now()
		emit(events.Event{Type: events.ToolCallStarted, Timestamp: start, Payload: events.ToolCallStartedPayload{ToolName: call.Name, Input: inputSanitized, StartedAt: start}})
		executable = append(executable, executableToolCall{index: i, call: call, tool: tool, inputSanitized: inputSanitized, startedAt: start})
	}

	if len(executable) == 0 {
		messages := collectToolMessagesAndRecords(calls, outcomes, result)
		result.StageTimingsMs[StageToolExecutionTotal] += durationMs(time.Since(batchStarted))
		return messages
	}

	if a.canRunParallelToolBatch(calls) {
		a.runParallelToolCalls(ctx, executable, repoRoot, outcomes, emit)
	} else {
		a.runSequentialToolCalls(ctx, executable, repoRoot, outcomes, emit)
	}

	messages := collectToolMessagesAndRecords(calls, outcomes, result)
	result.StageTimingsMs[StageToolExecutionTotal] += durationMs(time.Since(batchStarted))
	return messages
}

func (a *Agent) runSequentialToolCalls(
	ctx context.Context,
	calls []executableToolCall,
	repoRoot string,
	outcomes []toolBatchOutcome,
	emit func(events.Event),
) {
	for _, call := range calls {
		outcomes[call.index] = a.executeOneToolCall(ctx, call, repoRoot, emit)
	}
}

func (a *Agent) runParallelToolCalls(
	ctx context.Context,
	calls []executableToolCall,
	repoRoot string,
	outcomes []toolBatchOutcome,
	emit func(events.Event),
) {
	parallelism := a.cfg.ToolParallelism
	if parallelism <= 0 {
		parallelism = 1
	}
	if parallelism > len(calls) {
		parallelism = len(calls)
	}

	sem := make(chan struct{}, parallelism)
	var wg sync.WaitGroup
	var emitMu sync.Mutex
	for _, toolCall := range calls {
		call := toolCall
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			outcome := a.executeOneToolCall(ctx, call, repoRoot, func(event events.Event) {
				emitMu.Lock()
				defer emitMu.Unlock()
				emit(event)
			})
			outcomes[call.index] = outcome
		}()
	}
	wg.Wait()
}

func (a *Agent) executeOneToolCall(
	ctx context.Context,
	call executableToolCall,
	repoRoot string,
	emit func(events.Event),
) toolBatchOutcome {
	meta := tools.Meta{RepoRoot: repoRoot, UnsafeShell: a.cfg.UnsafeShell, ToolTimeoutSeconds: a.cfg.ToolTimeoutSeconds}
	switch call.call.Name {
	case "grep":
		meta.MaxResults = a.cfg.ToolLimits.GrepMaxResults
		meta.MaxBytes = a.cfg.ToolLimits.GrepMaxBytes
	case "shell":
		meta.MaxBytes = a.cfg.ToolLimits.ShellMaxBytes
	case "exa_search":
		meta.MaxBytes = a.cfg.ToolLimits.WebMaxBytes
	case "list_tree":
		meta.MaxBytes = a.cfg.ToolLimits.ContextMaxBytes
	}

	res, err := call.tool.Execute(ctx, call.call.Arguments, meta)
	duration := time.Since(call.startedAt).Milliseconds()
	if err != nil {
		payload := map[string]any{"error": err.Error(), "duration_ms": duration}
		record := ToolCallRecord{ToolName: call.call.Name, Input: call.inputSanitized, Output: payload, Status: "error", StartedAt: call.startedAt, DurationMs: duration}
		emit(events.Event{Type: events.ToolCallFailed, Timestamp: time.Now(), Payload: events.ToolCallFinishedPayload{ToolName: call.call.Name, Status: "error", Preview: err.Error(), DurationMs: duration, LineCount: 1, ByteCount: len(err.Error()), Truncated: false}})
		payloadBytes, _ := json.Marshal(payload)
		return toolBatchOutcome{record: &record, toolMessage: string(payloadBytes), executed: true, toolName: call.call.Name, durationMs: duration}
	}

	res.DurationMs = duration
	record := ToolCallRecord{ToolName: call.call.Name, Input: call.inputSanitized, Output: res.Payload, Status: "success", StartedAt: call.startedAt, DurationMs: duration}
	emit(events.Event{Type: events.ToolCallFinished, Timestamp: time.Now(), Payload: events.ToolCallFinishedPayload{
		ToolName:   call.call.Name,
		Status:     "success",
		Output:     res.Payload,
		Preview:    res.Preview,
		LineCount:  res.LineCount,
		ByteCount:  res.ByteCount,
		Truncated:  res.Truncated,
		DurationMs: duration,
	}})
	payloadBytes, _ := json.Marshal(res.Payload)
	return toolBatchOutcome{record: &record, toolMessage: string(payloadBytes), executed: true, toolName: call.call.Name, durationMs: duration}
}

func collectToolMessagesAndRecords(
	calls []llm.ToolCall,
	outcomes []toolBatchOutcome,
	result *RunResult,
) []openai.ChatCompletionMessageParamUnion {
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(calls))
	for i := range outcomes {
		outcome := outcomes[i]
		if outcome.executed {
			result.StageTimingsMs[StageToolExecutionPrefix+outcome.toolName] += outcome.durationMs
		}
		if outcome.record != nil {
			result.ToolCalls = append(result.ToolCalls, *outcome.record)
		}
		if outcome.toolMessage != "" {
			messages = append(messages, openai.ToolMessage(outcome.toolMessage, calls[i].ID))
		}
	}
	return messages
}

func (a *Agent) canRunParallelToolBatch(calls []llm.ToolCall) bool {
	if a.cfg.ToolParallelism <= 1 || len(calls) < 2 {
		return false
	}
	for _, call := range calls {
		switch call.Name {
		case "grep", "exa_search", "list_tree":
			continue
		default:
			return false
		}
	}
	return true
}
