package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ag-cli/internal/config"
	"ag-cli/internal/events"
	"ag-cli/internal/llm"
	"ag-cli/internal/render"
	"ag-cli/internal/repo"
	"ag-cli/internal/tools"
	"ag-cli/internal/util"
	"ag-cli/internal/version"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared/constant"
	"go.uber.org/zap"
)

// RunResult captures run output for JSON mode.
type RunResult struct {
	RunID       string           `json:"run_id"`
	StartedAt   time.Time        `json:"timestamp_start"`
	FinishedAt  time.Time        `json:"timestamp_end"`
	RepoRoot    string           `json:"repo_root"`
	Question    string           `json:"question"`
	Model       string           `json:"model"`
	StepsUsed   int              `json:"steps_used"`
	Status      string           `json:"status"`
	FinalAnswer string           `json:"final_answer"`
	ToolCalls   []ToolCallRecord `json:"tool_calls"`
	Events      []events.Event   `json:"events"`
}

// ToolCallRecord records tool call history.
type ToolCallRecord struct {
	ToolName   string    `json:"tool_name"`
	Input      any       `json:"input"`
	Output     any       `json:"output"`
	Status     string    `json:"status"`
	StartedAt  time.Time `json:"started_at"`
	DurationMs int64     `json:"duration_ms"`
}

// Agent runs the orchestration loop.
type Agent struct {
	client   llm.Client
	tools    *tools.Registry
	renderer render.Renderer
	logger   *zap.Logger
	cfg      config.Config
}

// NewAgent constructs an Agent.
func NewAgent(client llm.Client, toolsReg *tools.Registry, renderer render.Renderer, logger *zap.Logger, cfg config.Config) *Agent {
	return &Agent{client: client, tools: toolsReg, renderer: renderer, logger: logger, cfg: cfg}
}

// Run executes the agent loop.
func (a *Agent) Run(ctx context.Context, question string, repoRoot string, repoCtx repo.RepoContext) (RunResult, error) {
	started := time.Now()
	runID := uuid.NewString()
	result := RunResult{
		RunID:     runID,
		StartedAt: started,
		RepoRoot:  repoRoot,
		Question:  question,
		Model:     a.cfg.Model,
		Status:    "failure",
	}

	emit := func(event events.Event) {
		result.Events = append(result.Events, event)
		if a.renderer != nil {
			a.renderer.Emit(event)
		}
	}

	emit(events.Event{Type: events.RunStarted, Timestamp: time.Now(), Payload: events.RunStartedPayload{
		Version:   version.Version,
		RepoRoot:  repoRoot,
		Model:     a.cfg.Model,
		RunID:     runID,
		StartedAt: started,
	}})

	var plan []string
	if !a.cfg.NoPlan {
		plan = a.generatePlan(ctx, question, repoCtx)
		emit(events.Event{Type: events.PlanGenerated, Timestamp: time.Now(), Payload: events.PlanGeneratedPayload{Plan: plan}})
	}

	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt()),
		openai.DeveloperMessage(developerPrompt(a.tools.Names(), !a.cfg.NoWeb)),
		openai.DeveloperMessage("Repository context:\n" + repoCtx.Summary()),
	}
	if !a.cfg.NoPlan && len(plan) > 0 {
		messages = append(messages, openai.DeveloperMessage("Plan:\n"+formatPlan(plan)))
	}
	if !a.cfg.NoHistory && a.cfg.HistoryLines > 0 {
		history := util.LoadShellHistory(a.cfg.HistoryLines)
		if len(history) > 0 {
			messages = append(messages, openai.DeveloperMessage("Recent shell history (most recent last):\n- "+strings.Join(history, "\n- ")))
		}
	}
	messages = append(messages, openai.UserMessage(question))

	toolsDefs := a.tools.OpenAITools()
	toolChoice := openai.ChatCompletionToolChoiceOptionUnionParam{}
	if len(toolsDefs) > 0 {
		toolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: param.NewOpt("auto")}
	}

	steps := 0
	for steps < a.cfg.MaxSteps {
		steps++
		response, err := a.client.Create(ctx, llm.Request{Model: a.cfg.Model, Messages: messages, Tools: toolsDefs, ToolChoice: toolChoice})
		if err != nil {
			a.logger.Error("model request failed", zap.Error(err))
			emit(events.Event{Type: events.RunError, Timestamp: time.Now(), Payload: events.RunErrorPayload{Message: err.Error()}})
			result.Status = "failure"
			result.StepsUsed = steps
			result.FinishedAt = time.Now()
			return result, err
		}

		if len(response.ToolCalls) == 0 {
			finalAnswer := strings.TrimSpace(response.Content)
			if !a.cfg.JSON {
				streamed, err := a.streamFinal(ctx, llm.Request{Model: a.cfg.Model, Messages: messages, Tools: toolsDefs, ToolChoice: toolChoice}, emit)
				if err != nil {
					a.logger.Error("streaming failed", zap.Error(err))
				} else if strings.TrimSpace(streamed) != "" {
					finalAnswer = streamed
				}
			}
			result.FinalAnswer = strings.TrimSpace(finalAnswer)
			result.Status = "success"
			result.StepsUsed = steps
			result.FinishedAt = time.Now()
			emit(events.Event{Type: events.FinalAnswerReady, Timestamp: time.Now(), Payload: events.FinalAnswerPayload{Answer: result.FinalAnswer}})
			emit(events.Event{Type: events.RunFinished, Timestamp: time.Now(), Payload: events.RunFinishedPayload{Status: result.Status, FinishedAt: result.FinishedAt}})
			return result, nil
		}

		// append assistant message with tool calls
		toolCallParams := make([]openai.ChatCompletionMessageToolCallUnionParam, 0, len(response.ToolCalls))
		for _, call := range response.ToolCalls {
			toolCallParams = append(toolCallParams, openai.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
					ID: call.ID,
					Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name:      call.Name,
						Arguments: string(call.Arguments),
					},
					Type: constant.Function("function"),
				},
			})
		}
		assistant := openai.ChatCompletionAssistantMessageParam{ToolCalls: toolCallParams}
		messages = append(messages, openai.ChatCompletionMessageParamUnion{OfAssistant: &assistant})

		for _, call := range response.ToolCalls {
			tool, ok := a.tools.Get(call.Name)
			if !ok {
				err := fmt.Errorf("unknown tool: %s", call.Name)
				emit(events.Event{Type: events.ToolCallFailed, Timestamp: time.Now(), Payload: events.ToolCallFinishedPayload{ToolName: call.Name, Status: "error", Preview: err.Error(), DurationMs: 0, LineCount: 1, ByteCount: len(err.Error())}})
				payloadBytes, _ := json.Marshal(map[string]string{"error": err.Error()})
				messages = append(messages, openai.ToolMessage(string(payloadBytes), call.ID))
				continue
			}
			inputSanitized := sanitizeInput(call.Arguments)
			start := time.Now()
			emit(events.Event{Type: events.ToolCallStarted, Timestamp: start, Payload: events.ToolCallStartedPayload{ToolName: call.Name, Input: inputSanitized, StartedAt: start}})

			meta := tools.Meta{RepoRoot: repoRoot, UnsafeShell: a.cfg.UnsafeShell, ToolTimeoutSeconds: 10}
			switch call.Name {
			case "grep":
				meta.MaxResults = a.cfg.ToolLimits.GrepMaxResults
				meta.MaxBytes = a.cfg.ToolLimits.GrepMaxBytes
			case "shell":
				meta.MaxBytes = a.cfg.ToolLimits.ShellMaxBytes
			case "exa_search":
				meta.MaxBytes = a.cfg.ToolLimits.WebMaxBytes
			}

			res, err := tool.Execute(ctx, call.Arguments, meta)
			duration := time.Since(start).Milliseconds()
			if err != nil {
				payload := map[string]any{"error": err.Error(), "duration_ms": duration}
				record := ToolCallRecord{ToolName: call.Name, Input: inputSanitized, Output: payload, Status: "error", StartedAt: start, DurationMs: duration}
				result.ToolCalls = append(result.ToolCalls, record)
				emit(events.Event{Type: events.ToolCallFailed, Timestamp: time.Now(), Payload: events.ToolCallFinishedPayload{ToolName: call.Name, Status: "error", Preview: err.Error(), DurationMs: duration, LineCount: 1, ByteCount: len(err.Error()), Truncated: false}})
				payloadBytes, _ := json.Marshal(payload)
				messages = append(messages, openai.ToolMessage(string(payloadBytes), call.ID))
				continue
			}
			res.DurationMs = duration
			record := ToolCallRecord{ToolName: call.Name, Input: inputSanitized, Output: res.Payload, Status: "success", StartedAt: start, DurationMs: duration}
			result.ToolCalls = append(result.ToolCalls, record)

			emit(events.Event{Type: events.ToolCallFinished, Timestamp: time.Now(), Payload: events.ToolCallFinishedPayload{
				ToolName:   call.Name,
				Status:     "success",
				Output:     res.Payload,
				Preview:    res.Preview,
				LineCount:  res.LineCount,
				ByteCount:  res.ByteCount,
				Truncated:  res.Truncated,
				DurationMs: duration,
			}})

			payloadBytes, _ := json.Marshal(res.Payload)
			messages = append(messages, openai.ToolMessage(string(payloadBytes), call.ID))
		}
	}

	// max steps reached
	warning := "Max steps reached. Provide the best possible partial answer and include a warning."
	messages = append(messages, openai.DeveloperMessage(warning))
	finalAnswer := "Max steps reached; unable to complete."
	if !a.cfg.JSON {
		streamed, err := a.streamFinal(ctx, llm.Request{Model: a.cfg.Model, Messages: messages, Tools: toolsDefs, ToolChoice: toolChoice}, emit)
		if err == nil && strings.TrimSpace(streamed) != "" {
			finalAnswer = streamed
		}
	}
	if !strings.Contains(strings.ToLower(finalAnswer), "max steps") {
		finalAnswer = "Max steps reached. " + finalAnswer
	}
	result.FinalAnswer = strings.TrimSpace(finalAnswer)
	result.Status = "partial"
	result.StepsUsed = steps
	result.FinishedAt = time.Now()
	emit(events.Event{Type: events.FinalAnswerReady, Timestamp: time.Now(), Payload: events.FinalAnswerPayload{Answer: result.FinalAnswer}})
	emit(events.Event{Type: events.RunFinished, Timestamp: time.Now(), Payload: events.RunFinishedPayload{Status: result.Status, FinishedAt: result.FinishedAt}})
	return result, errors.New("max steps reached")
}

func (a *Agent) generatePlan(ctx context.Context, question string, repoCtx repo.RepoContext) []string {
	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt()),
		openai.DeveloperMessage(planPrompt()),
		openai.DeveloperMessage("Repository context:\n" + repoCtx.Summary()),
		openai.UserMessage(question),
	}
	resp, err := a.client.Create(ctx, llm.Request{Model: a.cfg.Model, Messages: messages})
	if err != nil {
		return []string{"Review repository context", "Run focused searches", "Summarize evidence with citations"}
	}
	return parsePlan(resp.Content)
}

func parsePlan(text string) []string {
	var plan []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimLeft(line, "-*")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(plan) < 8 {
			plan = append(plan, line)
		}
	}
	if len(plan) < 3 {
		plan = append(plan, "Review repository context", "Run targeted tool calls", "Produce cited answer")
	}
	return plan
}

func formatPlan(plan []string) string {
	var b strings.Builder
	for _, item := range plan {
		b.WriteString("- ")
		b.WriteString(item)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func (a *Agent) streamFinal(ctx context.Context, req llm.Request, emit func(events.Event)) (string, error) {
	var builder strings.Builder
	_, err := a.client.Stream(ctx, req, func(delta string) {
		emit(events.Event{Type: events.ModelDelta, Timestamp: time.Now(), Payload: events.ModelDeltaPayload{Delta: delta}})
		builder.WriteString(delta)
	})
	if err != nil {
		return builder.String(), err
	}
	return builder.String(), nil
}

func sanitizeInput(args json.RawMessage) any {
	if len(args) == 0 {
		return map[string]any{}
	}
	var data any
	if err := json.Unmarshal(args, &data); err != nil {
		return map[string]any{"raw": util.RedactSecrets(string(args))}
	}
	if bytes, err := json.Marshal(data); err == nil {
		return string(util.RedactSecrets(string(bytes)))
	}
	return data
}
