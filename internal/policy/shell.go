package policy

import (
	"bytes"
	"errors"
	"regexp"
	"strings"
)

type ShellMode string

const (
	ShellModeReadOnly  ShellMode = "read-only"
	ShellModeAllowlist ShellMode = "allowlist"
	ShellModeUnsafe    ShellMode = "unsafe"
)

type ShellDecision struct {
	Mode         ShellMode
	Allowed      bool
	Reason       string
	CommandName  string
	CommandParts []string
}

var (
	interactiveCommands = map[string]struct{}{
		"vim": {}, "vi": {}, "nano": {}, "less": {}, "more": {}, "man": {}, "top": {}, "htop": {}, "ssh": {}, "sftp": {},
	}
	networkCommands = map[string]struct{}{
		"curl": {}, "wget": {}, "ssh": {}, "scp": {}, "nc": {}, "netcat": {},
		"ping": {}, "dig": {}, "nslookup": {}, "whois": {}, "traceroute": {},
	}
	destructivePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\brm\b`),
		regexp.MustCompile(`(?i)\bmkfs\b`),
		regexp.MustCompile(`(?i)\bdd\b`),
		regexp.MustCompile(`(?i)\bshutdown\b`),
		regexp.MustCompile(`(?i)\breboot\b`),
		regexp.MustCompile(`(?i)\bkill\s+-9\b`),
		regexp.MustCompile(`(?i):\(\)\{`),
		regexp.MustCompile(`(?i)chmod\s+-R\s+777\s+/`),
		regexp.MustCompile(`(?i)(>|>>)[\s]*(/etc|/bin|/usr|/var|/lib|/sbin|/System|/Library)`),
	}
)

func ResolveShellMode(unsafeShell bool, allowlist []string) ShellMode {
	if unsafeShell {
		return ShellModeUnsafe
	}
	if len(NormalizeAllowlist(allowlist)) > 0 {
		return ShellModeAllowlist
	}
	return ShellModeReadOnly
}

func EvaluateShellCommand(command string, unsafeShell bool, allowlist []string) ShellDecision {
	mode := ResolveShellMode(unsafeShell, allowlist)
	decision := ShellDecision{Mode: mode}

	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		decision.Reason = "command is required"
		return decision
	}
	cmdParts, err := SplitCommand(trimmed)
	if err != nil {
		decision.Reason = err.Error()
		return decision
	}
	if len(cmdParts) == 0 {
		decision.Reason = "command is required"
		return decision
	}
	decision.CommandParts = cmdParts
	decision.CommandName = cmdParts[0]
	cmdKey := strings.ToLower(cmdParts[0])

	if _, ok := interactiveCommands[cmdKey]; ok {
		decision.Reason = "interactive commands are not allowed"
		return decision
	}

	if mode == ShellModeReadOnly {
		decision.Reason = "shell is disabled in read-only mode"
		return decision
	}
	if mode == ShellModeUnsafe {
		decision.Allowed = true
		decision.Reason = "allowed (unsafe mode)"
		return decision
	}

	normalizedAllowlist := NormalizeAllowlist(allowlist)
	if len(normalizedAllowlist) == 0 {
		decision.Reason = "shell allowlist is empty"
		return decision
	}
	if !isAllowlisted(cmdParts, normalizedAllowlist) {
		decision.Reason = "command not allowlisted"
		return decision
	}
	if _, ok := networkCommands[cmdKey]; ok {
		decision.Reason = "network commands are blocked by default"
		return decision
	}
	for _, re := range destructivePatterns {
		if re.MatchString(trimmed) {
			decision.Reason = "blocked potentially destructive command"
			return decision
		}
	}

	decision.Allowed = true
	decision.Reason = "allowed"
	return decision
}

func NormalizeAllowlist(list []string) [][]string {
	out := make([][]string, 0, len(list))
	for _, item := range list {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		tokens, err := SplitCommand(trimmed)
		if err != nil || len(tokens) == 0 {
			continue
		}
		for i := range tokens {
			tokens[i] = strings.ToLower(tokens[i])
		}
		out = append(out, tokens)
	}
	return out
}

func SplitCommand(input string) ([]string, error) {
	var args []string
	var buf bytes.Buffer
	inSingle := false
	inDouble := false
	escape := false

	for _, r := range input {
		if escape {
			buf.WriteRune(r)
			escape = false
			continue
		}
		if r == '\\' && !inSingle {
			escape = true
			continue
		}
		if r == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if r == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if (r == ' ' || r == '\t' || r == '\n') && !inSingle && !inDouble {
			if buf.Len() > 0 {
				args = append(args, buf.String())
				buf.Reset()
			}
			continue
		}
		buf.WriteRune(r)
	}
	if escape || inSingle || inDouble {
		return nil, errors.New("unterminated quote or escape in command")
	}
	if buf.Len() > 0 {
		args = append(args, buf.String())
	}
	return args, nil
}

func isAllowlisted(cmdParts []string, allowlist [][]string) bool {
	if len(allowlist) == 0 || len(cmdParts) == 0 {
		return false
	}
	normalized := make([]string, len(cmdParts))
	for i, part := range cmdParts {
		normalized[i] = strings.ToLower(part)
	}
	for _, entry := range allowlist {
		if len(normalized) < len(entry) {
			continue
		}
		match := true
		for i := range entry {
			if normalized[i] != entry[i] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
