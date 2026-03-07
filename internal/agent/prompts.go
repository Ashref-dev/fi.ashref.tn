package agent

import (
	"fmt"
	"strings"
)

func systemPrompt(responseMode string) string {
	modeGuidance := "Keep final responses concise and practical."
	switch strings.ToLower(strings.TrimSpace(responseMode)) {
	case "operator":
		modeGuidance = "Respond in operator mode: list exact command(s), then one-line caveats."
	case "explain":
		modeGuidance = "Respond in explain mode: concise bullets with short rationale and citations."
	default:
		modeGuidance = "Respond in quick mode: 1-3 short lines unless safety requires more detail."
	}
	return strings.TrimSpace(fmt.Sprintf(`You are fi-cli, a terminal-native agent for answering repository questions.

Requirements:
- Use tools to find evidence rather than guessing.
- Do not reveal chain-of-thought. Provide short, factual answers.
- Respond in plain text. Be concise unless the user asks for more detail.
- Default behavior is read-only; only use tools listed. If shell is available, use it only for explicitly allowlisted, read-only commands.
- When the user asks for a command or how to do something, prioritize finding the exact command(s) in repo files and return them clearly.
- If evidence is missing, say so explicitly and explain what would be needed.
- Never invent file paths or dependencies.
- Cite evidence inline using [path:line] for file evidence and [tool:<name>] for tool outputs.
- %s`, modeGuidance))
}

func developerPrompt(toolNames []string, webEnabled bool, shellAllowlist []string, commandIntent bool) string {
	webNote := "Web search is available via exa_search."
	if !webEnabled {
		webNote = "Web search is unavailable; do not request exa_search."
	}
	shellNote := "Shell tool is disabled; do not request shell commands."
	if contains(toolNames, "shell") {
		if len(shellAllowlist) > 0 {
			shellNote = "Shell allowlist prefixes: " + strings.Join(shellAllowlist, ", ") + ". Only run commands that match these prefixes."
		} else {
			shellNote = "Shell tool is enabled but no allowlist is configured; avoid requesting shell."
		}
	}
	intentNote := "General query mode."
	if commandIntent {
		intentNote = "Command-intent mode: prioritize exact runnable commands from repository evidence."
	}
	return strings.TrimSpace(fmt.Sprintf(`You can call tools: %s.
%s
%s
%s

Tool usage rules:
- Keep tool inputs minimal and focused.
- Respect truncation; if results are incomplete, call tools again with narrower queries.
- Prefer grep before shell commands.
- For command-intent questions, search in this order:
  1) package.json scripts, Makefile, Justfile
  2) README and docs (setup/run/deploy sections)
  3) docker-compose, Dockerfile, CI files, infra folders
- When returning commands, include the exact command first, then source citation.

Final answer format:
- Start with a brief summary.
- Include evidence citations inline.
- End with actionable next steps if relevant.
`, strings.Join(toolNames, ", "), webNote, shellNote, intentNote))
}

func planPrompt() string {
	return strings.TrimSpace(`Generate a concise plan of 3-8 bullets describing intended actions. Do not include reasoning or tool outputs.`)
}

func contains(list []string, target string) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}

func isCommandIntent(question string) bool {
	query := strings.ToLower(question)
	keywords := []string{
		"how do i run", "how to run", "run this", "start", "dev server", "build", "test",
		"deploy", "docker", "kubernetes", "kubectl", "aws", "azure", "gcp",
		"command", "cli", "script", "make", "npm run", "pnpm", "bun", "ci",
		"push", "release", "migrate", "seed",
	}
	for _, keyword := range keywords {
		if strings.Contains(query, keyword) {
			return true
		}
	}
	return false
}
