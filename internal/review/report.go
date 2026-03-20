package review

import (
	"fmt"
	"strings"
)

func FormatText(result Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "review: base=%s head=%s merge_base=%s\n", result.BaseRef, result.HeadRef, result.MergeBase)
	fmt.Fprintf(&b, "score: %d/5\n", result.Score5)
	if result.MergeReady {
		fmt.Fprintln(&b, "merge_ready: yes")
	} else {
		fmt.Fprintln(&b, "merge_ready: no")
	}
	fmt.Fprintf(&b, "summary: %s\n", result.Summary)

	fmt.Fprintln(&b, "\nblockers:")
	blockers := filterFindingsBySeverity(result.Findings, "blocker")
	if len(blockers) == 0 {
		fmt.Fprintln(&b, "none")
	} else {
		for _, finding := range blockers {
			fmt.Fprintf(&b, "- %s:%d %s\n", finding.File, finding.LineStart, finding.Title)
			fmt.Fprintf(&b, "  %s\n", finding.Body)
		}
	}

	fmt.Fprintln(&b, "\nfindings:")
	for _, severity := range []string{"high", "medium", "low"} {
		items := filterFindingsBySeverity(result.Findings, severity)
		if len(items) == 0 {
			continue
		}
		fmt.Fprintf(&b, "%s:\n", severity)
		for _, finding := range items {
			fmt.Fprintf(&b, "- %s:%d %s\n", finding.File, finding.LineStart, finding.Title)
			fmt.Fprintf(&b, "  %s\n", finding.Body)
			if finding.SuggestedFix != "" {
				fmt.Fprintf(&b, "  fix: %s\n", finding.SuggestedFix)
			}
		}
	}
	if len(result.Findings) == 0 {
		fmt.Fprintln(&b, "none")
	}

	fmt.Fprintln(&b, "\nstrengths:")
	if len(result.Strengths) == 0 {
		fmt.Fprintln(&b, "none")
	} else {
		for _, strength := range result.Strengths {
			fmt.Fprintf(&b, "- %s\n", strength)
		}
	}

	fmt.Fprintln(&b, "\ncoverage and uncertainty:")
	fmt.Fprintf(&b, "reviewed_files: %d\n", result.Coverage.ReviewedCount)
	fmt.Fprintf(&b, "skipped_files: %d\n", result.Coverage.SkippedCount)
	if result.Coverage.BinaryCount > 0 {
		fmt.Fprintf(&b, "binary_files: %d\n", result.Coverage.BinaryCount)
	}
	if result.Coverage.OverSizedCount > 0 {
		fmt.Fprintf(&b, "oversized_files: %d\n", result.Coverage.OverSizedCount)
	}
	if len(result.SkippedFiles) > 0 {
		fmt.Fprintln(&b, "skipped:")
		for _, file := range result.SkippedFiles {
			fmt.Fprintf(&b, "- %s\n", file)
		}
	}
	if len(result.Coverage.Uncertainty) > 0 {
		fmt.Fprintln(&b, "uncertainty:")
		for _, item := range result.Coverage.Uncertainty {
			fmt.Fprintf(&b, "- %s\n", item)
		}
	}
	if len(result.Coverage.Notes) > 0 {
		fmt.Fprintln(&b, "notes:")
		for _, note := range result.Coverage.Notes {
			fmt.Fprintf(&b, "- %s\n", note)
		}
	}

	return strings.TrimSpace(b.String())
}

func filterFindingsBySeverity(findings []Finding, severity string) []Finding {
	items := make([]Finding, 0)
	for _, finding := range findings {
		if normalizeSeverity(finding.Severity) == severity {
			items = append(items, finding)
		}
	}
	return items
}
