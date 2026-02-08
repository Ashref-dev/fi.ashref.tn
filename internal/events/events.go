package events

import "time"

// Type represents an emitted event type.
type Type string

const (
	RunStarted       Type = "RunStarted"
	PlanGenerated    Type = "PlanGenerated"
	ToolCallStarted  Type = "ToolCallStarted"
	ToolCallFinished Type = "ToolCallFinished"
	ToolCallFailed   Type = "ToolCallFailed"
	ModelDelta       Type = "ModelStreamingDelta"
	FinalAnswerReady Type = "FinalAnswerReady"
	RunFinished      Type = "RunFinished"
	RunError         Type = "RunError"
)

// Event is the common envelope for renderer events.
type Event struct {
	Type      Type      `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Payload   any       `json:"payload"`
}

// RunStartedPayload is emitted at the beginning of a run.
type RunStartedPayload struct {
	Version   string    `json:"version"`
	RepoRoot  string    `json:"repo_root"`
	Model     string    `json:"model"`
	RunID     string    `json:"run_id"`
	StartedAt time.Time `json:"started_at"`
}

// PlanGeneratedPayload contains the model plan.
type PlanGeneratedPayload struct {
	Plan []string `json:"plan"`
}

// ToolCallStartedPayload marks tool call start.
type ToolCallStartedPayload struct {
	ToolName  string    `json:"tool_name"`
	Input     any       `json:"input"`
	StartedAt time.Time `json:"started_at"`
}

// ToolCallFinishedPayload marks tool call end.
type ToolCallFinishedPayload struct {
	ToolName   string `json:"tool_name"`
	Status     string `json:"status"`
	Output     any    `json:"output"`
	Preview    string `json:"preview"`
	LineCount  int    `json:"line_count"`
	ByteCount  int    `json:"byte_count"`
	Truncated  bool   `json:"truncated"`
	DurationMs int64  `json:"duration_ms"`
}

// ModelDeltaPayload is streamed as tokens arrive.
type ModelDeltaPayload struct {
	Delta string `json:"delta"`
}

// FinalAnswerPayload is emitted when final answer is ready.
type FinalAnswerPayload struct {
	Answer string `json:"answer"`
}

// RunFinishedPayload closes the run.
type RunFinishedPayload struct {
	Status     string    `json:"status"`
	FinishedAt time.Time `json:"finished_at"`
}

// RunErrorPayload records a run error.
type RunErrorPayload struct {
	Message string `json:"message"`
}
