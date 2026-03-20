package review

import (
	"sort"
	"strings"
	"time"
)

type Params struct {
	BaseRef     string
	HeadRef     string
	WorkingTree bool
}

type Coverage struct {
	TotalChanged        int      `json:"total_changed"`
	ReviewedCount       int      `json:"reviewed_count"`
	SkippedCount        int      `json:"skipped_count"`
	BinaryCount         int      `json:"binary_count"`
	OverSizedCount      int      `json:"oversized_count"`
	WorkingTreeIncluded bool     `json:"working_tree_included"`
	Notes               []string `json:"notes"`
	Uncertainty         []string `json:"uncertainty"`
}

type Finding struct {
	Severity     string  `json:"severity"`
	Category     string  `json:"category"`
	Confidence   float64 `json:"confidence"`
	Title        string  `json:"title"`
	Body         string  `json:"body"`
	File         string  `json:"file"`
	LineStart    int     `json:"line_start"`
	LineEnd      int     `json:"line_end"`
	Evidence     string  `json:"evidence"`
	SuggestedFix string  `json:"suggested_fix"`
}

type Result struct {
	RunID          string           `json:"run_id"`
	StartedAt      time.Time        `json:"timestamp_start"`
	FinishedAt     time.Time        `json:"timestamp_end"`
	RepoRoot       string           `json:"repo_root"`
	Model          string           `json:"model"`
	BaseRef        string           `json:"base_ref"`
	HeadRef        string           `json:"head_ref"`
	MergeBase      string           `json:"merge_base"`
	Score5         int              `json:"score_5"`
	MergeReady     bool             `json:"merge_ready"`
	BlockerCount   int              `json:"blocker_count"`
	Summary        string           `json:"summary"`
	Strengths      []string         `json:"strengths"`
	Coverage       Coverage         `json:"coverage"`
	SkippedFiles   []string         `json:"skipped_files"`
	ReviewedFiles  []string         `json:"reviewed_files"`
	Findings       []Finding        `json:"findings"`
	StageTimingsMs map[string]int64 `json:"stage_timings_ms"`
}

type fileReviewResponse struct {
	Summary     string    `json:"summary"`
	Strengths   []string  `json:"strengths"`
	Uncertainty []string  `json:"uncertainty"`
	Findings    []Finding `json:"findings"`
}

func severityRank(severity string) int {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "blocker":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func NormalizeFinding(f Finding, fallbackPath string) Finding {
	f.Severity = normalizeSeverity(f.Severity)
	f.Category = normalizeCategory(f.Category)
	f.Title = strings.TrimSpace(f.Title)
	f.Body = strings.TrimSpace(f.Body)
	f.File = strings.TrimSpace(f.File)
	if f.File == "" {
		f.File = fallbackPath
	}
	f.Evidence = strings.TrimSpace(f.Evidence)
	f.SuggestedFix = strings.TrimSpace(f.SuggestedFix)
	if f.LineStart < 0 {
		f.LineStart = 0
	}
	if f.LineEnd < f.LineStart {
		f.LineEnd = f.LineStart
	}
	if f.Confidence < 0 {
		f.Confidence = 0
	}
	if f.Confidence > 1 {
		f.Confidence = 1
	}
	return f
}

func normalizeSeverity(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "blocker":
		return "blocker"
	case "high":
		return "high"
	case "medium":
		return "medium"
	default:
		return "low"
	}
}

func normalizeCategory(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "correctness", "security", "performance", "concurrency", "compatibility", "tests", "data-loss", "api":
		return value
	case "":
		return "correctness"
	default:
		return value
	}
}

func ScoreFindings(findings []Finding) int {
	score := 5
	for _, finding := range findings {
		switch normalizeSeverity(finding.Severity) {
		case "blocker":
			return 1
		case "high":
			if score > 2 {
				score = 2
			}
		case "medium":
			if score > 3 {
				score = 3
			}
		case "low":
			if score > 4 {
				score = 4
			}
		}
	}
	return score
}

func SortFindings(findings []Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		left := findings[i]
		right := findings[j]
		if severityRank(left.Severity) != severityRank(right.Severity) {
			return severityRank(left.Severity) > severityRank(right.Severity)
		}
		if left.Confidence != right.Confidence {
			return left.Confidence > right.Confidence
		}
		if left.File != right.File {
			return left.File < right.File
		}
		if left.LineStart != right.LineStart {
			return left.LineStart < right.LineStart
		}
		return left.Title < right.Title
	})
}

func CountBlockers(findings []Finding) int {
	count := 0
	for _, finding := range findings {
		if normalizeSeverity(finding.Severity) == "blocker" {
			count++
		}
	}
	return count
}
