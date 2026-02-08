package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"ag-cli/internal/util"
)

type ShellTool struct{}

// NewShellTool constructs a shell tool.
func NewShellTool() *ShellTool { return &ShellTool{} }

func (s *ShellTool) Name() string { return "shell" }

func (s *ShellTool) Description() string {
	return "Run a safe local shell command with allowlist and timeouts."
}

func (s *ShellTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{"type": "string"},
			"cwd":     map[string]any{"type": "string"},
		},
		"required":             []string{"command"},
		"additionalProperties": false,
	}
}

type shellInput struct {
	Command string `json:"command"`
	Cwd     string `json:"cwd"`
}

type shellOutput struct {
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   int    `json:"exit_code"`
	DurationMs int64  `json:"duration_ms"`
	Truncated  bool   `json:"truncated"`
}

var (
	allowlist = map[string]struct{}{
		"rg": {}, "ls": {}, "cat": {}, "sed": {}, "awk": {}, "head": {}, "tail": {}, "git": {}, "find": {}, "pwd": {}, "tree": {},
		"go": {}, "node": {}, "npm": {}, "pnpm": {}, "yarn": {}, "bun": {}, "python": {}, "pip": {}, "make": {},
	}
	interactive = map[string]struct{}{
		"vim": {}, "vi": {}, "nano": {}, "less": {}, "more": {}, "man": {}, "top": {}, "htop": {}, "ssh": {}, "sftp": {},
	}
	networkTools = map[string]struct{}{
		"curl": {}, "wget": {}, "ssh": {}, "scp": {}, "nc": {}, "netcat": {},
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

func (s *ShellTool) Execute(ctx context.Context, input json.RawMessage, meta Meta) (Result, error) {
	var args shellInput
	if err := json.Unmarshal(input, &args); err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(args.Command) == "" {
		return Result{}, errors.New("command is required")
	}

	cmdParts, err := splitCommand(args.Command)
	if err != nil {
		return Result{}, err
	}
	if len(cmdParts) == 0 {
		return Result{}, errors.New("command is required")
	}
	cmdName := cmdParts[0]

	if _, ok := interactive[cmdName]; ok {
		return Result{}, fmt.Errorf("interactive commands are not allowed: %s", cmdName)
	}

	if !meta.UnsafeShell {
		if _, ok := allowlist[cmdName]; !ok {
			return Result{}, fmt.Errorf("command not allowlisted: %s", cmdName)
		}
		if _, ok := networkTools[cmdName]; ok {
			return Result{}, fmt.Errorf("network commands are blocked by default: %s", cmdName)
		}
		for _, re := range destructivePatterns {
			if re.MatchString(args.Command) {
				return Result{}, fmt.Errorf("blocked potentially destructive command")
			}
		}
	}

	cwd := meta.RepoRoot
	if strings.TrimSpace(args.Cwd) != "" {
		resolved, err := resolveCwd(meta.RepoRoot, args.Cwd)
		if err != nil {
			return Result{}, err
		}
		cwd = resolved
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(meta.ToolTimeoutSeconds)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, cmdName, cmdParts[1:]...)
	cmd.Dir = cwd
	cmd.Env = minimalEnv()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	start := time.Now()
	err = cmd.Run()
	duration := time.Since(start).Milliseconds()

	exitCode := 0
	if err != nil {
		if exitErr := (&exec.ExitError{}); errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return Result{}, err
		}
	}

	outStr := util.RedactSecrets(stdout.String())
	errStr := util.RedactSecrets(stderr.String())
	truncated := false
	if meta.MaxBytes > 0 {
		if trimmed, did := util.TruncateBytes(outStr, meta.MaxBytes); did {
			outStr = trimmed
			truncated = true
		}
		if trimmed, did := util.TruncateBytes(errStr, meta.MaxBytes); did {
			errStr = trimmed
			truncated = true
		}
	}

	output := shellOutput{
		Stdout:     outStr,
		Stderr:     errStr,
		ExitCode:   exitCode,
		DurationMs: duration,
		Truncated:  truncated,
	}
	preview := util.Preview(strings.TrimSpace(outStr+"\n"+errStr), 12, 2000)
	lineCount := 0
	if preview != "" {
		lineCount = strings.Count(preview, "\n") + 1
	}
	byteCount := len(outStr) + len(errStr)
	return Result{ToolName: s.Name(), Payload: output, Preview: preview, LineCount: lineCount, ByteCount: byteCount, Truncated: truncated, DurationMs: duration}, nil
}

func resolveCwd(repoRoot, cwd string) (string, error) {
	if filepath.IsAbs(cwd) {
		rel, err := filepath.Rel(repoRoot, cwd)
		if err != nil || strings.HasPrefix(rel, "..") {
			return "", fmt.Errorf("cwd must stay within repo root")
		}
		return cwd, nil
	}
	abs := filepath.Join(repoRoot, cwd)
	rel, err := filepath.Rel(repoRoot, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("cwd must stay within repo root")
	}
	return abs, nil
}

func minimalEnv() []string {
	if runtime.GOOS == "windows" {
		return nil
	}
	return nil
}

func splitCommand(input string) ([]string, error) {
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
