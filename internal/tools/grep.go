package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"fi-cli/internal/repo"
	"fi-cli/internal/util"
)

type GrepTool struct {
	rgPath string
}

// NewGrepTool constructs a grep tool that prefers ripgrep.
func NewGrepTool() *GrepTool {
	path, _ := exec.LookPath("rg")
	return &GrepTool{rgPath: path}
}

func (g *GrepTool) Name() string { return "grep" }

func (g *GrepTool) Description() string {
	return "Search for a regex pattern in repository files using ripgrep when available."
}

func (g *GrepTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{"type": "string"},
			"paths": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"glob": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"case_sensitive": map[string]any{"type": "boolean"},
			"max_results":    map[string]any{"type": "integer", "minimum": 1},
		},
		"required":             []string{"pattern"},
		"additionalProperties": false,
	}
}

type grepInput struct {
	Pattern       string   `json:"pattern"`
	Paths         []string `json:"paths"`
	Glob          []string `json:"glob"`
	CaseSensitive bool     `json:"case_sensitive"`
	MaxResults    int      `json:"max_results"`
}

type grepOutput struct {
	Matches    []string `json:"matches"`
	Truncated  bool     `json:"truncated"`
	DurationMs int64    `json:"duration_ms"`
	Warning    string   `json:"warning,omitempty"`
}

func (g *GrepTool) Execute(ctx context.Context, input json.RawMessage, meta Meta) (Result, error) {
	var args grepInput
	if err := json.Unmarshal(input, &args); err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(args.Pattern) == "" {
		return Result{}, errors.New("pattern is required")
	}
	if args.MaxResults <= 0 {
		args.MaxResults = meta.MaxResults
	}

	start := time.Now()
	if g.rgPath != "" {
		matches, warning, err := g.runRipgrep(ctx, args, meta)
		if err != nil {
			return Result{}, err
		}
		redacted := redactLines(matches)
		lines, truncated, byteCount := util.TruncateLinesAndBytes(redacted, args.MaxResults, meta.MaxBytes)
		output := grepOutput{Matches: lines, Truncated: truncated, DurationMs: time.Since(start).Milliseconds(), Warning: warning}
		preview := util.Preview(strings.Join(lines, "\n"), 12, 2000)
		return Result{ToolName: g.Name(), Payload: output, Preview: preview, LineCount: len(lines), ByteCount: byteCount, Truncated: truncated, DurationMs: output.DurationMs}, nil
	}

	matches, err := g.runFallback(ctx, args, meta)
	if err != nil {
		return Result{}, err
	}
	redacted := redactLines(matches)
	lines, truncated, byteCount := util.TruncateLinesAndBytes(redacted, args.MaxResults, meta.MaxBytes)
	output := grepOutput{Matches: lines, Truncated: truncated, DurationMs: time.Since(start).Milliseconds(), Warning: "rg not found; using Go fallback"}
	preview := util.Preview(strings.Join(lines, "\n"), 12, 2000)
	return Result{ToolName: g.Name(), Payload: output, Preview: preview, LineCount: len(lines), ByteCount: byteCount, Truncated: truncated, DurationMs: output.DurationMs}, nil
}

func (g *GrepTool) runRipgrep(ctx context.Context, args grepInput, meta Meta) ([]string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(meta.ToolTimeoutSeconds)*time.Second)
	defer cancel()

	cmdArgs := []string{"--no-heading", "--line-number"}
	if !args.CaseSensitive {
		cmdArgs = append(cmdArgs, "--ignore-case")
	}
	for _, glob := range args.Glob {
		if strings.TrimSpace(glob) == "" {
			continue
		}
		cmdArgs = append(cmdArgs, "--glob", glob)
	}
	for _, deny := range denylistGlobs() {
		cmdArgs = append(cmdArgs, "--glob", deny)
	}
	cmdArgs = append(cmdArgs, args.Pattern)

	paths := sanitizePaths(args.Paths, meta.RepoRoot)
	if len(paths) == 0 {
		paths = []string{"."}
	}
	cmdArgs = append(cmdArgs, paths...)

	cmd := exec.CommandContext(ctx, g.rgPath, cmdArgs...)
	cmd.Dir = meta.RepoRoot
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		exitErr := &exec.ExitError{}
		if errors.As(err, &exitErr) {
			if exitErr.ExitCode() == 1 { // no matches
				return []string{}, "", nil
			}
		}
		return nil, "", fmt.Errorf("rg failed: %w: %s", err, stderr.String())
	}

	lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return []string{}, "", nil
	}
	return lines, "", nil
}

func (g *GrepTool) runFallback(ctx context.Context, args grepInput, meta Meta) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(meta.ToolTimeoutSeconds)*time.Second)
	defer cancel()
	stopWalk := errors.New("stop-walk")

	pattern := args.Pattern
	if !args.CaseSensitive {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	paths := sanitizePaths(args.Paths, meta.RepoRoot)
	if len(paths) == 0 {
		paths = []string{meta.RepoRoot}
	}

	var matches []string
	for _, root := range paths {
		select {
		case <-ctx.Done():
			return matches, ctx.Err()
		default:
		}
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
					return filepath.SkipDir
				}
				return nil
			}
			if repo.IsDenylisted(path) {
				return nil
			}
			if len(args.Glob) > 0 && !matchAnyGlob(path, meta.RepoRoot, args.Glob) {
				return nil
			}
			file, err := os.Open(path)
			if err != nil {
				return nil
			}
			defer file.Close()
			if isBinary(file) {
				return nil
			}
			_, _ = file.Seek(0, io.SeekStart)
			scanner := bufio.NewScanner(file)
			lineNum := 1
			for scanner.Scan() {
				line := scanner.Text()
				if re.MatchString(line) {
					rel, _ := filepath.Rel(meta.RepoRoot, path)
					matches = append(matches, fmt.Sprintf("%s:%d:%s", rel, lineNum, line))
					if args.MaxResults > 0 && len(matches) >= args.MaxResults {
						return stopWalk
					}
				}
				lineNum++
			}
			return nil
		})
		if err != nil {
			if errors.Is(err, stopWalk) {
				return matches, nil
			}
			return matches, err
		}
	}
	return matches, nil
}

func sanitizePaths(paths []string, repoRoot string) []string {
	var out []string
	for _, p := range paths {
		if p == "" {
			continue
		}
		abs := p
		if !filepath.IsAbs(p) {
			abs = filepath.Join(repoRoot, p)
		}
		rel, err := filepath.Rel(repoRoot, abs)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		out = append(out, rel)
	}
	return out
}

func denylistGlobs() []string {
	return []string{
		"!.env*",
		"!*.pem",
		"!*.key",
		"!*.p12",
		"!*.pfx",
		"!id_rsa*",
		"!.aws/credentials",
		"!.npmrc",
		"!.docker/config.json",
	}
}

func matchAnyGlob(pathValue string, root string, globs []string) bool {
	rel, err := filepath.Rel(root, pathValue)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	for _, g := range globs {
		clean := strings.ReplaceAll(g, "**", "*")
		if ok, _ := path.Match(clean, rel); ok {
			return true
		}
	}
	return false
}

func isBinary(file *os.File) bool {
	buf := make([]byte, 8000)
	n, _ := file.Read(buf)
	for _, b := range buf[:n] {
		if b == 0 {
			return true
		}
	}
	return false
}

func redactLines(lines []string) []string {
	if len(lines) == 0 {
		return lines
	}
	redacted := make([]string, 0, len(lines))
	for _, line := range lines {
		redacted = append(redacted, util.RedactSecrets(line))
	}
	return redacted
}
