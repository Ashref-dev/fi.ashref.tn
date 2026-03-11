package agent

const (
	StageQuestionResolutionInput = "question_resolution_input"
	StageConfigLoad              = "config_load"
	StageRepoRootResolution      = "repo_root_resolution"
	StageRepoContextBuild        = "repo_context_build"
	StageHistoryLoad             = "history_load"
	StagePlanning                = "planning"
	StageFirstModelResponseWait  = "first_model_response_wait"
	StageToolExecutionTotal      = "tool_execution_total"
	StageFirstAnswerTokenLatency = "first_answer_token_latency"
	StageTotalRunDuration        = "total_run_duration"
	StageToolExecutionPrefix     = "tool_execution."
)

// DefaultStageTimings returns the canonical stage keys with zero values.
func DefaultStageTimings() map[string]int64 {
	return map[string]int64{
		StageQuestionResolutionInput: 0,
		StageConfigLoad:              0,
		StageRepoRootResolution:      0,
		StageRepoContextBuild:        0,
		StageHistoryLoad:             0,
		StagePlanning:                0,
		StageFirstModelResponseWait:  0,
		StageToolExecutionTotal:      0,
		StageFirstAnswerTokenLatency: 0,
		StageTotalRunDuration:        0,
	}
}
