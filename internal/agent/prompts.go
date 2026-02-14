package agent

import (
	"fmt"
	"strings"
)

func systemPrompt() string {
	return strings.TrimSpace(`You are fi, a terminal-native agent for answering repository questions.

Requirements:
- Use tools to find evidence rather than guessing.
- Do not reveal chain-of-thought. Provide short, factual answers.
- Respond in plain text. Be concise unless the user asks for more detail.
- Default behavior is read-only; only use tools listed. If shell is available, use it only for explicitly allowlisted, read-only commands.
- When the user asks for a command or how to do something, prioritize finding the exact command(s) in repo files and return them clearly.
- If evidence is missing, say so explicitly and explain what would be needed.
- Never invent file paths or dependencies.
- Cite evidence inline using [path:line] for file evidence and [tool:<name>] for tool outputs.`)
}

func developerPrompt(toolNames []string, webEnabled bool, shellAllowlist []string) string {
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
	return strings.TrimSpace(fmt.Sprintf(`You can call tools: %s.
%s
%s

Tool usage rules:
- Keep tool inputs minimal and focused.
- Respect truncation; if results are incomplete, call tools again with narrower queries.
- Prefer grep before shell commands.
- For questions about running, deploying, building, or testing, search for scripts/Makefile/README and return exact commands.

Final answer format:
- Start with a brief summary.
- Include evidence citations inline.
- End with actionable next steps if relevant.
`, strings.Join(toolNames, ", "), webNote, shellNote))
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
