package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"fi-cli/internal/policy"
	"fi-cli/internal/util"
)

type ShellTool struct {
	allowlist []string
}

// NewShellTool constructs a shell tool.
func NewShellTool(allowlist []string) *ShellTool {
	return &ShellTool{allowlist: allowlist}
}

func (s *ShellTool) Name() string { return "shell" }

func (s *ShellTool) Description() string {
	return "Run a local shell command from the configured allowlist with timeouts."
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

func (s *ShellTool) Execute(ctx context.Context, input json.RawMessage, meta Meta) (Result, error) {
	var args shellInput
	if err := json.Unmarshal(input, &args); err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(args.Command) == "" {
		return Result{}, errors.New("command is required")
	}

	decision := policy.EvaluateShellCommand(args.Command, meta.UnsafeShell, s.allowlist)
	if !decision.Allowed {
		return Result{}, errors.New(decision.Reason)
	}
	cmdParts := decision.CommandParts
	cmdName := decision.CommandName

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
	err := cmd.Run()
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
