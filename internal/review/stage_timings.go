package review

const (
	StageRefResolution    = "ref_resolution"
	StageMergeBase        = "merge_base_resolution"
	StageChangedFilesScan = "changed_files_scan"
	StageDiffContextBuild = "diff_context_build"
	StageTriage           = "triage"
	StageDeepReview       = "deep_review"
	StageModelWaitTotal   = "model_wait_total"
	StageAggregation      = "aggregation"
	StageTotalRunDuration = "total_run_duration"
)

func DefaultStageTimings() map[string]int64 {
	return map[string]int64{
		StageRefResolution:    0,
		StageMergeBase:        0,
		StageChangedFilesScan: 0,
		StageDiffContextBuild: 0,
		StageTriage:           0,
		StageDeepReview:       0,
		StageModelWaitTotal:   0,
		StageAggregation:      0,
		StageTotalRunDuration: 0,
	}
}
