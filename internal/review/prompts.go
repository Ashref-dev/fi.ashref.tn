package review

import (
	"fmt"
	"strings"
)

func reviewSystemPrompt() string {
	return strings.TrimSpace(`You are fi-cli review, a senior code reviewer operating on a local Git branch diff.

Requirements:
- Focus on correctness, regressions, security, data loss, API compatibility, concurrency, performance, and missing tests.
- Ignore trivial style comments unless they hide a real defect.
- Comment only on changed lines or directly affected surrounding behavior.
- Report uncertainty explicitly. If evidence is insufficient, say so instead of guessing.
- Return JSON only. Do not wrap the JSON in markdown or prose.
- Every finding must include severity, category, confidence, title, body, file, line_start, line_end, evidence, and suggested_fix.
- Severity must be one of: blocker, high, medium, low.
- Confidence must be between 0 and 1.
- Use low severity for minor issues. Use blocker only when merge should stop until fixed.`)
}

func reviewDeveloperPrompt(input reviewPromptInput) string {
	return strings.TrimSpace(fmt.Sprintf(`Review this changed file in repository context.

Review rules:
- Behave like a senior reviewer.
- Prefer concrete, high-signal findings.
- Avoid style-only comments.
- Only cite evidence from the provided diff/context.
- If there are no real issues, return an empty findings array.

Output schema:
{
  "summary": "short file review summary",
  "strengths": ["short strength"],
  "uncertainty": ["short uncertainty"],
  "findings": [
    {
      "severity": "blocker|high|medium|low",
      "category": "correctness|security|performance|concurrency|compatibility|tests|data-loss|api",
      "confidence": 0.0,
      "title": "short title",
      "body": "why this matters",
      "file": "%s",
      "line_start": 0,
      "line_end": 0,
      "evidence": "path:line or diff hunk reference",
      "suggested_fix": "specific fix"
    }
  ]
}

Repository summary:
%s

Top-level tree snapshot:
%s

Global review instructions:
%s

Path-specific review instructions:
%s

Changed file:
Path: %s
Status: %s
Additions: %d
Deletions: %d

Diff:
%s

Head file context:
%s

Base file context:
%s

Targeted grep evidence:
%s

Touched-line blame:
%s

Recent commit subjects in range:
%s
`,
		input.Path,
		input.RepoSummary,
		input.TreeSnapshot,
		emptyFallback(input.GlobalInstructions, "(none)"),
		emptyFallback(input.PathInstructions, "(none)"),
		input.Path,
		input.Status,
		input.Additions,
		input.Deletions,
		emptyFallback(input.DiffPatch, "(no diff available)"),
		emptyFallback(input.HeadContext, "(missing)"),
		emptyFallback(input.BaseContext, "(missing)"),
		emptyFallback(input.GrepEvidence, "(none)"),
		emptyFallback(input.BlameEvidence, "(none)"),
		emptyFallback(input.LogEvidence, "(none)"),
	))
}

type reviewPromptInput struct {
	Path               string
	Status             string
	Additions          int
	Deletions          int
	RepoSummary        string
	TreeSnapshot       string
	GlobalInstructions string
	PathInstructions   string
	DiffPatch          string
	HeadContext        string
	BaseContext        string
	GrepEvidence       string
	BlameEvidence      string
	LogEvidence        string
}

func emptyFallback(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
