package agent

import (
	"strings"
	"testing"
)

func TestSystemPromptEnforcesStrictPlainTextPolicy(t *testing.T) {
	prompt := systemPrompt("quick")
	checks := []string{
		"Respond in plain text only",
		"No markdown",
		"no emojis",
		"return the exact command(s) first",
		"Do not use markdown links",
	}
	for _, check := range checks {
		if !strings.Contains(prompt, check) {
			t.Fatalf("expected system prompt to contain %q", check)
		}
	}
}

func TestDeveloperPromptCommandIntentIsCommandFirst(t *testing.T) {
	prompt := developerPrompt([]string{"grep", "shell"}, true, []string{"git status"}, true)
	checks := []string{
		"Plain text only; no markdown and no emojis",
		"first line is the exact runnable command(s)",
		"not markdown links",
		"run a quick ls or ls -la at repo root",
	}
	for _, check := range checks {
		if !strings.Contains(prompt, check) {
			t.Fatalf("expected developer prompt to contain %q", check)
		}
	}
}
