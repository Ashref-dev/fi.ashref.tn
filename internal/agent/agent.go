package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"fi-cli/internal/config"
	"fi-cli/internal/events"
	"fi-cli/internal/llm"
	"fi-cli/internal/render"
	"fi-cli/internal/repo"
	"fi-cli/internal/tools"
	"fi-cli/internal/util"
	"fi-cli/internal/version"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared/constant"
	"go.uber.org/zap"
)

// RunResult captures run output for JSON mode.
type RunResult struct {
	RunID          string           `json:"run_id"`
	StartedAt      time.Time        `json:"timestamp_start"`
	FinishedAt     time.Time        `json:"timestamp_end"`
	RepoRoot       string           `json:"repo_root"`
	Question       string           `json:"question"`
	Model          string           `json:"model"`
	StepsUsed      int              `json:"steps_used"`
	Status         string           `json:"status"`
	FinalAnswer    string           `json:"final_answer"`
	StageTimingsMs map[string]int64 `json:"stage_timings_ms"`
	ToolCalls      []ToolCallRecord `json:"tool_calls"`
	Events         []events.Event   `json:"events"`
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
		RunID:          runID,
		StartedAt:      started,
		RepoRoot:       repoRoot,
		Question:       question,
		Model:          a.cfg.Model,
		Status:         "failure",
		StageTimingsMs: DefaultStageTimings(),
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
	firstAnswerTokenCaptured := false
	captureFirstAnswerToken := func(mark time.Time) {
		if firstAnswerTokenCaptured {
			return
		}
		result.StageTimingsMs[StageFirstAnswerTokenLatency] = durationMs(mark.Sub(started))
		firstAnswerTokenCaptured = true
	}
	commandIntent := isCommandIntent(question)
	if !a.cfg.NoPlan {
		planningStart := time.Now()
		plan = a.generatePlan(ctx, question, repoCtx)
		result.StageTimingsMs[StagePlanning] = durationMs(time.Since(planningStart))
		emit(events.Event{Type: events.PlanGenerated, Timestamp: time.Now(), Payload: events.PlanGeneratedPayload{Plan: plan}})
	}

	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt(a.cfg.ResponseMode)),
		openai.DeveloperMessage(developerPrompt(a.tools.Names(), !a.cfg.NoWeb, a.cfg.ShellAllowlist, commandIntent)),
		openai.DeveloperMessage("Repository context:\n" + repoCtx.Summary()),
	}
	if !a.cfg.NoPlan && len(plan) > 0 {
		messages = append(messages, openai.DeveloperMessage("Plan:\n"+formatPlan(plan)))
	}
	if !a.cfg.NoHistory && a.cfg.HistoryLines > 0 {
		historyStart := time.Now()
		history := util.LoadShellHistory(a.cfg.HistoryLines)
		result.StageTimingsMs[StageHistoryLoad] = durationMs(time.Since(historyStart))
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
	toolUsage := map[string]int{}
	a.captureInitialStructureSnapshot(ctx, repoRoot, toolUsage, &result, &messages, emit)
	firstModelResponseCaptured := false
	for steps < a.cfg.MaxSteps {
		steps++
		modelWaitStart := time.Now()
		modelRequest := llm.Request{Model: a.cfg.Model, Messages: messages, Tools: toolsDefs, ToolChoice: toolChoice}
		var (
			response     llm.Response
			err          error
			firstDeltaAt time.Time
		)
		if a.cfg.JSON {
			response, err = a.client.Create(ctx, modelRequest)
		} else {
			response, firstDeltaAt, err = a.streamModelStep(ctx, modelRequest, emit)
		}
		if !firstModelResponseCaptured {
			if !firstDeltaAt.IsZero() {
				result.StageTimingsMs[StageFirstModelResponseWait] = durationMs(firstDeltaAt.Sub(modelWaitStart))
			} else {
				result.StageTimingsMs[StageFirstModelResponseWait] = durationMs(time.Since(modelWaitStart))
			}
			firstModelResponseCaptured = true
		}
		if err != nil {
			a.logger.Error("model request failed", zap.Error(err))
			emit(events.Event{Type: events.RunError, Timestamp: time.Now(), Payload: events.RunErrorPayload{Message: err.Error()}})
			result.Status = "failure"
			result.StepsUsed = steps
			result.FinishedAt = time.Now()
			result.StageTimingsMs[StageTotalRunDuration] = durationMs(result.FinishedAt.Sub(started))
			return result, err
		}

		if len(response.ToolCalls) == 0 {
			if a.shouldRequireGrepEvidence(toolUsage) {
				messages = append(messages, openai.DeveloperMessage("Before final answer, run at least one focused grep call using question terms, then answer with evidence references."))
				continue
			}
			if !firstDeltaAt.IsZero() {
				captureFirstAnswerToken(firstDeltaAt)
			} else if strings.TrimSpace(response.Content) != "" {
				captureFirstAnswerToken(time.Now())
			}
			finalAnswer := strings.TrimSpace(response.Content)
			result.FinalAnswer = strings.TrimSpace(finalAnswer)
			result.Status = "success"
			result.StepsUsed = steps
			result.FinishedAt = time.Now()
			result.StageTimingsMs[StageTotalRunDuration] = durationMs(result.FinishedAt.Sub(started))
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
		toolMessages := a.executeToolBatch(ctx, response.ToolCalls, repoRoot, toolUsage, &result, emit)
		messages = append(messages, toolMessages...)
	}

	// max steps reached
	warning := "Max steps reached. Provide the best possible partial answer and include a warning."
	messages = append(messages, openai.DeveloperMessage(warning))
	finalAnswer := "Max steps reached; unable to complete."
	if !a.cfg.JSON {
		response, firstDeltaAt, err := a.streamModelStep(ctx, llm.Request{Model: a.cfg.Model, Messages: messages, Tools: toolsDefs, ToolChoice: toolChoice}, emit)
		if err == nil && strings.TrimSpace(response.Content) != "" {
			if !firstDeltaAt.IsZero() {
				captureFirstAnswerToken(firstDeltaAt)
			}
			finalAnswer = response.Content
		}
	}
	if !strings.Contains(strings.ToLower(finalAnswer), "max steps") {
		finalAnswer = "Max steps reached. " + finalAnswer
	}
	result.FinalAnswer = strings.TrimSpace(finalAnswer)
	result.Status = "partial"
	result.StepsUsed = steps
	result.FinishedAt = time.Now()
	result.StageTimingsMs[StageTotalRunDuration] = durationMs(result.FinishedAt.Sub(started))
	emit(events.Event{Type: events.FinalAnswerReady, Timestamp: time.Now(), Payload: events.FinalAnswerPayload{Answer: result.FinalAnswer}})
	emit(events.Event{Type: events.RunFinished, Timestamp: time.Now(), Payload: events.RunFinishedPayload{Status: result.Status, FinishedAt: result.FinishedAt}})
	return result, errors.New("max steps reached")
}

func (a *Agent) shouldRequireGrepEvidence(toolUsage map[string]int) bool {
	if _, ok := a.tools.Get("grep"); !ok {
		return false
	}
	if toolUsage["grep"] > 0 {
		return false
	}
	// If grep cannot run (for example explicit zero call budget), do not deadlock.
	return a.withinToolBudget("grep", toolUsage)
}

func (a *Agent) captureInitialStructureSnapshot(
	ctx context.Context,
	repoRoot string,
	toolUsage map[string]int,
	result *RunResult,
	messages *[]openai.ChatCompletionMessageParamUnion,
	emit func(events.Event),
) {
	tool, ok := a.tools.Get("list_tree")
	if !ok {
		return
	}

	args, _ := json.Marshal(map[string]any{
		"path":        ".",
		"max_depth":   2,
		"max_entries": 200,
	})
	inputSanitized := sanitizeInput(args)
	start := time.Now()
	emit(events.Event{Type: events.ToolCallStarted, Timestamp: start, Payload: events.ToolCallStartedPayload{
		ToolName:  "list_tree",
		Input:     inputSanitized,
		StartedAt: start,
	}})

	meta := tools.Meta{
		RepoRoot:           repoRoot,
		UnsafeShell:        a.cfg.UnsafeShell,
		ToolTimeoutSeconds: a.cfg.ToolTimeoutSeconds,
		MaxBytes:           a.cfg.ToolLimits.ContextMaxBytes,
	}

	res, err := tool.Execute(ctx, args, meta)
	toolUsage["list_tree"]++
	duration := time.Since(start).Milliseconds()
	result.StageTimingsMs[StageToolExecutionTotal] += duration
	result.StageTimingsMs[StageToolExecutionPrefix+"list_tree"] += duration

	if err != nil {
		payload := map[string]any{"error": err.Error(), "duration_ms": duration}
		result.ToolCalls = append(result.ToolCalls, ToolCallRecord{
			ToolName:   "list_tree",
			Input:      inputSanitized,
			Output:     payload,
			Status:     "error",
			StartedAt:  start,
			DurationMs: duration,
		})
		emit(events.Event{Type: events.ToolCallFailed, Timestamp: time.Now(), Payload: events.ToolCallFinishedPayload{
			ToolName:   "list_tree",
			Status:     "error",
			Preview:    err.Error(),
			LineCount:  1,
			ByteCount:  len(err.Error()),
			Truncated:  false,
			DurationMs: duration,
		}})
		return
	}

	res.DurationMs = duration
	result.ToolCalls = append(result.ToolCalls, ToolCallRecord{
		ToolName:   "list_tree",
		Input:      inputSanitized,
		Output:     res.Payload,
		Status:     "success",
		StartedAt:  start,
		DurationMs: duration,
	})
	emit(events.Event{Type: events.ToolCallFinished, Timestamp: time.Now(), Payload: events.ToolCallFinishedPayload{
		ToolName:   "list_tree",
		Status:     "success",
		Output:     res.Payload,
		Preview:    res.Preview,
		LineCount:  res.LineCount,
		ByteCount:  res.ByteCount,
		Truncated:  res.Truncated,
		DurationMs: duration,
	}})

	snapshot := strings.TrimSpace(util.Preview(res.Preview, 80, 8000))
	if snapshot == "" {
		return
	}
	*messages = append(*messages, openai.DeveloperMessage("Repository structure snapshot (auto list_tree at run start):\n"+snapshot))
}

func (a *Agent) generatePlan(ctx context.Context, question string, repoCtx repo.RepoContext) []string {
	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt(a.cfg.ResponseMode)),
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

func (a *Agent) streamModelStep(ctx context.Context, req llm.Request, emit func(events.Event)) (llm.Response, time.Time, error) {
	var firstDeltaAt time.Time
	response, err := a.client.Stream(ctx, req, func(delta string) {
		if firstDeltaAt.IsZero() && delta != "" {
			firstDeltaAt = time.Now()
		}
		emit(events.Event{Type: events.ModelDelta, Timestamp: time.Now(), Payload: events.ModelDeltaPayload{Delta: delta}})
	})
	if err != nil {
		return response, firstDeltaAt, err
	}
	return response, firstDeltaAt, nil
}

func (a *Agent) withinToolBudget(toolName string, usage map[string]int) bool {
	current := usage[toolName]
	switch toolName {
	case "grep":
		return current < a.cfg.ToolLimits.GrepMaxCalls
	case "shell":
		return current < a.cfg.ToolLimits.ShellMaxCalls
	case "exa_search":
		return current < a.cfg.ToolLimits.WebMaxCalls
	default:
		return true
	}
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

func durationMs(d time.Duration) int64 {
	if d < 0 {
		return 0
	}
	return d.Milliseconds()
}
