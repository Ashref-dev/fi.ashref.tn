package review

import (
	"strings"
	"testing"
)

func TestReviewPromptsIncludeRubricAndInstructions(t *testing.T) {
	systemPrompt := reviewSystemPrompt()
	for _, fragment := range []string{
		"Focus on correctness, regressions, security, data loss, API compatibility, concurrency, performance, and missing tests.",
		"Ignore trivial style comments unless they hide a real defect.",
		"Return JSON only.",
	} {
		if !strings.Contains(systemPrompt, fragment) {
			t.Fatalf("expected system prompt to contain %q", fragment)
		}
	}

	developerPrompt := reviewDeveloperPrompt(reviewPromptInput{
		Path:               "internal/service/auth.go",
		Status:             "M",
		Additions:          12,
		Deletions:          3,
		RepoSummary:        "Go CLI with review mode",
		TreeSnapshot:       "./\n├── cmd/\n└── internal/",
		GlobalInstructions: "Always verify auth behavior and tests.",
		PathInstructions:   "Focus on API compatibility for auth files.",
		DiffPatch:          "@@ -1,2 +1,3 @@\n- old\n+ new",
		HeadContext:        "new code",
		BaseContext:        "old code",
		GrepEvidence:       "auth.go:12:HandleLogin",
		BlameEvidence:      "12 abc123 previous auth change",
		LogEvidence:        "abc123 previous auth refactor",
	})
	for _, fragment := range []string{
		"Avoid style-only comments.",
		"Always verify auth behavior and tests.",
		"Focus on API compatibility for auth files.",
		"\"severity\": \"blocker|high|medium|low\"",
		"Path: internal/service/auth.go",
		"Targeted grep evidence:",
	} {
		if !strings.Contains(developerPrompt, fragment) {
			t.Fatalf("expected developer prompt to contain %q", fragment)
		}
	}
}
