package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"fi-cli/internal/util"
)

var gitDiffHunkHeaderPattern = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

type GitRefCandidate struct {
	Ref         string `json:"ref"`
	ResolvedRef string `json:"resolved_ref,omitempty"`
	Commit      string `json:"commit,omitempty"`
	Exists      bool   `json:"exists"`
	Error       string `json:"error,omitempty"`
}

type GitRefsOutput struct {
	HeadRef    string            `json:"head_ref"`
	Candidates []GitRefCandidate `json:"candidates"`
	DurationMs int64             `json:"duration_ms"`
}

type GitMergeBaseOutput struct {
	BaseRef    string `json:"base_ref"`
	HeadRef    string `json:"head_ref"`
	MergeBase  string `json:"merge_base"`
	DurationMs int64  `json:"duration_ms"`
}

type GitChangedFile struct {
	Path        string `json:"path"`
	OldPath     string `json:"old_path,omitempty"`
	Status      string `json:"status"`
	Additions   int    `json:"additions"`
	Deletions   int    `json:"deletions"`
	Binary      bool   `json:"binary"`
	WorkingTree bool   `json:"working_tree"`
}

type GitChangedFilesOutput struct {
	BaseRef      string           `json:"base_ref"`
	HeadRef      string           `json:"head_ref"`
	WorkingTree  bool             `json:"working_tree"`
	ChangedFiles []GitChangedFile `json:"changed_files"`
	DurationMs   int64            `json:"duration_ms"`
}

type GitDiffHunk struct {
	OldStart     int   `json:"old_start"`
	OldLines     int   `json:"old_lines"`
	NewStart     int   `json:"new_start"`
	NewLines     int   `json:"new_lines"`
	AddedLines   []int `json:"added_lines,omitempty"`
	DeletedLines []int `json:"deleted_lines,omitempty"`
}

type GitDiffHunksOutput struct {
	Path       string        `json:"path"`
	OldPath    string        `json:"old_path,omitempty"`
	Patch      string        `json:"patch"`
	Hunks      []GitDiffHunk `json:"hunks"`
	Truncated  bool          `json:"truncated"`
	DurationMs int64         `json:"duration_ms"`
}

type GitFileAtRefOutput struct {
	Path       string `json:"path"`
	Ref        string `json:"ref,omitempty"`
	Content    string `json:"content"`
	Missing    bool   `json:"missing"`
	Truncated  bool   `json:"truncated"`
	DurationMs int64  `json:"duration_ms"`
}

type GitBlameLine struct {
	Commit   string `json:"commit"`
	Author   string `json:"author,omitempty"`
	Summary  string `json:"summary,omitempty"`
	Line     int    `json:"line"`
	LineText string `json:"line_text,omitempty"`
}

type GitBlameOutput struct {
	Path       string         `json:"path"`
	Ref        string         `json:"ref"`
	StartLine  int            `json:"start_line"`
	EndLine    int            `json:"end_line"`
	Lines      []GitBlameLine `json:"lines"`
	DurationMs int64          `json:"duration_ms"`
}

type GitLogEntry struct {
	Commit  string `json:"commit"`
	Subject string `json:"subject"`
}

type GitLogRangeOutput struct {
	BaseRef    string        `json:"base_ref"`
	HeadRef    string        `json:"head_ref"`
	Path       string        `json:"path,omitempty"`
	Entries    []GitLogEntry `json:"entries"`
	DurationMs int64         `json:"duration_ms"`
}

type GitRefsTool struct{}
type GitMergeBaseTool struct{}
type GitChangedFilesTool struct{}
type GitDiffHunksTool struct{}
type GitFileAtRefTool struct{}
type GitBlameLinesTool struct{}
type GitLogRangeTool struct{}

func NewGitRefsTool() *GitRefsTool                 { return &GitRefsTool{} }
func NewGitMergeBaseTool() *GitMergeBaseTool       { return &GitMergeBaseTool{} }
func NewGitChangedFilesTool() *GitChangedFilesTool { return &GitChangedFilesTool{} }
func NewGitDiffHunksTool() *GitDiffHunksTool       { return &GitDiffHunksTool{} }
func NewGitFileAtRefTool() *GitFileAtRefTool       { return &GitFileAtRefTool{} }
func NewGitBlameLinesTool() *GitBlameLinesTool     { return &GitBlameLinesTool{} }
func NewGitLogRangeTool() *GitLogRangeTool         { return &GitLogRangeTool{} }

func (t *GitRefsTool) Name() string         { return "git_refs" }
func (t *GitMergeBaseTool) Name() string    { return "git_merge_base" }
func (t *GitChangedFilesTool) Name() string { return "git_changed_files" }
func (t *GitDiffHunksTool) Name() string    { return "git_diff_hunks" }
func (t *GitFileAtRefTool) Name() string    { return "git_file_at_ref" }
func (t *GitBlameLinesTool) Name() string   { return "git_blame_lines" }
func (t *GitLogRangeTool) Name() string     { return "git_log_range" }

func (t *GitRefsTool) Description() string {
	return "Resolve Git refs and candidate refs in a repository."
}

func (t *GitMergeBaseTool) Description() string {
	return "Compute the merge base between two Git refs."
}

func (t *GitChangedFilesTool) Description() string {
	return "List changed files between Git refs, including status and binary detection."
}

func (t *GitDiffHunksTool) Description() string {
	return "Read unified diff hunks for a single changed file between Git refs."
}

func (t *GitFileAtRefTool) Description() string {
	return "Read file contents at a Git ref or from the working tree."
}

func (t *GitBlameLinesTool) Description() string {
	return "Read Git blame metadata for a line range."
}

func (t *GitLogRangeTool) Description() string {
	return "Read commit subjects across a Git range."
}

func (t *GitRefsTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"candidates": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
		},
		"additionalProperties": false,
	}
}

func (t *GitMergeBaseTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"base_ref": map[string]any{"type": "string"},
			"head_ref": map[string]any{"type": "string"},
		},
		"required":             []string{"base_ref", "head_ref"},
		"additionalProperties": false,
	}
}

func (t *GitChangedFilesTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"base_ref":     map[string]any{"type": "string"},
			"head_ref":     map[string]any{"type": "string"},
			"working_tree": map[string]any{"type": "boolean"},
		},
		"required":             []string{"base_ref"},
		"additionalProperties": false,
	}
}

func (t *GitDiffHunksTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"base_ref":     map[string]any{"type": "string"},
			"head_ref":     map[string]any{"type": "string"},
			"path":         map[string]any{"type": "string"},
			"old_path":     map[string]any{"type": "string"},
			"working_tree": map[string]any{"type": "boolean"},
		},
		"required":             []string{"base_ref", "path"},
		"additionalProperties": false,
	}
}

func (t *GitFileAtRefTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ref":          map[string]any{"type": "string"},
			"path":         map[string]any{"type": "string"},
			"working_tree": map[string]any{"type": "boolean"},
		},
		"required":             []string{"path"},
		"additionalProperties": false,
	}
}

func (t *GitBlameLinesTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ref":        map[string]any{"type": "string"},
			"path":       map[string]any{"type": "string"},
			"start_line": map[string]any{"type": "integer", "minimum": 1},
			"end_line":   map[string]any{"type": "integer", "minimum": 1},
		},
		"required":             []string{"ref", "path", "start_line", "end_line"},
		"additionalProperties": false,
	}
}

func (t *GitLogRangeTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"base_ref": map[string]any{"type": "string"},
			"head_ref": map[string]any{"type": "string"},
			"path":     map[string]any{"type": "string"},
			"limit":    map[string]any{"type": "integer", "minimum": 1, "maximum": 50},
		},
		"required":             []string{"base_ref", "head_ref"},
		"additionalProperties": false,
	}
}

func (t *GitRefsTool) Execute(ctx context.Context, input json.RawMessage, meta Meta) (Result, error) {
	start := time.Now()
	var args struct {
		Candidates []string `json:"candidates"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &args); err != nil {
			return Result{}, err
		}
	}

	headRef, _ := gitReadOneLine(ctx, meta, "rev-parse", "--abbrev-ref", "HEAD")
	output := GitRefsOutput{HeadRef: headRef}
	for _, candidate := range args.Candidates {
		output.Candidates = append(output.Candidates, resolveGitCandidate(ctx, meta, candidate))
	}
	output.DurationMs = time.Since(start).Milliseconds()
	payloadBytes, _ := json.Marshal(output)
	return Result{
		ToolName:   t.Name(),
		Payload:    output,
		Preview:    util.Preview(string(payloadBytes), 12, 2000),
		LineCount:  len(output.Candidates) + 1,
		ByteCount:  len(payloadBytes),
		Truncated:  false,
		DurationMs: output.DurationMs,
	}, nil
}

func (t *GitMergeBaseTool) Execute(ctx context.Context, input json.RawMessage, meta Meta) (Result, error) {
	start := time.Now()
	var args struct {
		BaseRef string `json:"base_ref"`
		HeadRef string `json:"head_ref"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(args.BaseRef) == "" || strings.TrimSpace(args.HeadRef) == "" {
		return Result{}, errors.New("base_ref and head_ref are required")
	}

	mergeBase, err := gitReadOneLine(ctx, meta, "merge-base", args.BaseRef, args.HeadRef)
	if err != nil {
		return Result{}, err
	}
	output := GitMergeBaseOutput{
		BaseRef:    args.BaseRef,
		HeadRef:    args.HeadRef,
		MergeBase:  mergeBase,
		DurationMs: time.Since(start).Milliseconds(),
	}
	payloadBytes, _ := json.Marshal(output)
	return Result{
		ToolName:   t.Name(),
		Payload:    output,
		Preview:    mergeBase,
		LineCount:  1,
		ByteCount:  len(payloadBytes),
		Truncated:  false,
		DurationMs: output.DurationMs,
	}, nil
}

func (t *GitChangedFilesTool) Execute(ctx context.Context, input json.RawMessage, meta Meta) (Result, error) {
	start := time.Now()
	var args struct {
		BaseRef     string `json:"base_ref"`
		HeadRef     string `json:"head_ref"`
		WorkingTree bool   `json:"working_tree"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(args.BaseRef) == "" {
		return Result{}, errors.New("base_ref is required")
	}
	if strings.TrimSpace(args.HeadRef) == "" {
		args.HeadRef = "HEAD"
	}

	entries, err := readChangedFiles(ctx, meta, args.BaseRef, args.HeadRef, args.WorkingTree)
	if err != nil {
		return Result{}, err
	}
	output := GitChangedFilesOutput{
		BaseRef:      args.BaseRef,
		HeadRef:      args.HeadRef,
		WorkingTree:  args.WorkingTree,
		ChangedFiles: entries,
		DurationMs:   time.Since(start).Milliseconds(),
	}
	payloadBytes, _ := json.Marshal(output)
	return Result{
		ToolName:   t.Name(),
		Payload:    output,
		Preview:    summarizeChangedFiles(entries),
		LineCount:  len(entries),
		ByteCount:  len(payloadBytes),
		Truncated:  false,
		DurationMs: output.DurationMs,
	}, nil
}

func (t *GitDiffHunksTool) Execute(ctx context.Context, input json.RawMessage, meta Meta) (Result, error) {
	start := time.Now()
	var args struct {
		BaseRef     string `json:"base_ref"`
		HeadRef     string `json:"head_ref"`
		Path        string `json:"path"`
		OldPath     string `json:"old_path"`
		WorkingTree bool   `json:"working_tree"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(args.BaseRef) == "" || strings.TrimSpace(args.Path) == "" {
		return Result{}, errors.New("base_ref and path are required")
	}
	if strings.TrimSpace(args.HeadRef) == "" {
		args.HeadRef = "HEAD"
	}
	relPath, err := sanitizeRepoRelativePath(meta.RepoRoot, args.Path)
	if err != nil {
		return Result{}, err
	}
	oldPath := strings.TrimSpace(args.OldPath)
	if oldPath != "" {
		oldPath, err = sanitizeRepoRelativePath(meta.RepoRoot, oldPath)
		if err != nil {
			return Result{}, err
		}
	}

	patch, err := readDiffPatch(ctx, meta, args.BaseRef, args.HeadRef, relPath, oldPath, args.WorkingTree)
	if err != nil {
		return Result{}, err
	}
	hunks := parseGitDiffHunks(patch)
	output := GitDiffHunksOutput{
		Path:       relPath,
		OldPath:    oldPath,
		Patch:      patch,
		Hunks:      hunks,
		DurationMs: time.Since(start).Milliseconds(),
	}
	payloadBytes, _ := json.Marshal(output)
	if meta.MaxBytes > 0 && len(payloadBytes) > meta.MaxBytes {
		output.Truncated = true
		output.Patch, _ = util.TruncateBytes(output.Patch, meta.MaxBytes/2)
		payloadBytes, _ = json.Marshal(output)
	}
	return Result{
		ToolName:   t.Name(),
		Payload:    output,
		Preview:    util.Preview(output.Patch, 40, 4000),
		LineCount:  strings.Count(strings.TrimSuffix(output.Patch, "\n"), "\n") + btoi(output.Patch != ""),
		ByteCount:  len(payloadBytes),
		Truncated:  output.Truncated,
		DurationMs: output.DurationMs,
	}, nil
}

func (t *GitFileAtRefTool) Execute(ctx context.Context, input json.RawMessage, meta Meta) (Result, error) {
	start := time.Now()
	var args struct {
		Ref         string `json:"ref"`
		Path        string `json:"path"`
		WorkingTree bool   `json:"working_tree"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(args.Path) == "" {
		return Result{}, errors.New("path is required")
	}
	relPath, err := sanitizeRepoRelativePath(meta.RepoRoot, args.Path)
	if err != nil {
		return Result{}, err
	}

	content, missing, err := readFileAtRef(ctx, meta, strings.TrimSpace(args.Ref), relPath, args.WorkingTree)
	if err != nil {
		return Result{}, err
	}
	output := GitFileAtRefOutput{
		Path:       relPath,
		Ref:        strings.TrimSpace(args.Ref),
		Content:    content,
		Missing:    missing,
		DurationMs: time.Since(start).Milliseconds(),
	}
	payloadBytes, _ := json.Marshal(output)
	if meta.MaxBytes > 0 && len(payloadBytes) > meta.MaxBytes {
		output.Truncated = true
		output.Content, _ = util.TruncateBytes(output.Content, meta.MaxBytes/2)
		payloadBytes, _ = json.Marshal(output)
	}
	return Result{
		ToolName:   t.Name(),
		Payload:    output,
		Preview:    util.Preview(output.Content, 40, 4000),
		LineCount:  strings.Count(strings.TrimSuffix(output.Content, "\n"), "\n") + btoi(output.Content != ""),
		ByteCount:  len(payloadBytes),
		Truncated:  output.Truncated,
		DurationMs: output.DurationMs,
	}, nil
}

func (t *GitBlameLinesTool) Execute(ctx context.Context, input json.RawMessage, meta Meta) (Result, error) {
	start := time.Now()
	var args struct {
		Ref       string `json:"ref"`
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(args.Ref) == "" || strings.TrimSpace(args.Path) == "" {
		return Result{}, errors.New("ref and path are required")
	}
	if args.StartLine <= 0 || args.EndLine < args.StartLine {
		return Result{}, errors.New("invalid line range")
	}
	relPath, err := sanitizeRepoRelativePath(meta.RepoRoot, args.Path)
	if err != nil {
		return Result{}, err
	}

	lines, err := readBlameLines(ctx, meta, args.Ref, relPath, args.StartLine, args.EndLine)
	if err != nil {
		return Result{}, err
	}
	output := GitBlameOutput{
		Path:       relPath,
		Ref:        args.Ref,
		StartLine:  args.StartLine,
		EndLine:    args.EndLine,
		Lines:      lines,
		DurationMs: time.Since(start).Milliseconds(),
	}
	payloadBytes, _ := json.Marshal(output)
	return Result{
		ToolName:   t.Name(),
		Payload:    output,
		Preview:    summarizeBlame(lines),
		LineCount:  len(lines),
		ByteCount:  len(payloadBytes),
		Truncated:  false,
		DurationMs: output.DurationMs,
	}, nil
}

func (t *GitLogRangeTool) Execute(ctx context.Context, input json.RawMessage, meta Meta) (Result, error) {
	start := time.Now()
	var args struct {
		BaseRef string `json:"base_ref"`
		HeadRef string `json:"head_ref"`
		Path    string `json:"path"`
		Limit   int    `json:"limit"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(args.BaseRef) == "" || strings.TrimSpace(args.HeadRef) == "" {
		return Result{}, errors.New("base_ref and head_ref are required")
	}
	if args.Limit <= 0 || args.Limit > 50 {
		args.Limit = 10
	}

	relPath := ""
	if strings.TrimSpace(args.Path) != "" {
		var err error
		relPath, err = sanitizeRepoRelativePath(meta.RepoRoot, args.Path)
		if err != nil {
			return Result{}, err
		}
	}
	entries, err := readLogRange(ctx, meta, args.BaseRef, args.HeadRef, relPath, args.Limit)
	if err != nil {
		return Result{}, err
	}
	output := GitLogRangeOutput{
		BaseRef:    args.BaseRef,
		HeadRef:    args.HeadRef,
		Path:       relPath,
		Entries:    entries,
		DurationMs: time.Since(start).Milliseconds(),
	}
	payloadBytes, _ := json.Marshal(output)
	return Result{
		ToolName:   t.Name(),
		Payload:    output,
		Preview:    summarizeLogEntries(entries),
		LineCount:  len(entries),
		ByteCount:  len(payloadBytes),
		Truncated:  false,
		DurationMs: output.DurationMs,
	}, nil
}

func resolveGitCandidate(ctx context.Context, meta Meta, candidate string) GitRefCandidate {
	out := GitRefCandidate{Ref: candidate}
	if strings.TrimSpace(candidate) == "" {
		out.Error = "candidate ref is empty"
		return out
	}

	resolvedRef := candidate
	if candidate == "origin/HEAD" {
		value, err := gitReadOneLine(ctx, meta, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
		if err != nil {
			out.Error = err.Error()
			return out
		}
		resolvedRef = value
	}
	out.ResolvedRef = resolvedRef
	commit, err := gitReadOneLine(ctx, meta, "rev-parse", "--verify", resolvedRef)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	out.Exists = true
	out.Commit = commit
	return out
}

func readChangedFiles(ctx context.Context, meta Meta, baseRef string, headRef string, workingTree bool) ([]GitChangedFile, error) {
	args := []string{"diff", "--name-status", "--find-renames", "--no-ext-diff"}
	if workingTree {
		args = append(args, baseRef)
	} else {
		args = append(args, baseRef+"..."+headRef)
	}
	stdout, err := gitReadBytes(ctx, meta, args...)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(stdout)), "\n")
	entries := make([]GitChangedFile, 0, len(lines))
	for _, raw := range lines {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		parts := strings.Split(raw, "\t")
		if len(parts) < 2 {
			continue
		}
		status := normalizeGitStatus(parts[0])
		entry := GitChangedFile{Status: status, WorkingTree: workingTree}
		switch status {
		case "R", "C":
			if len(parts) >= 3 {
				entry.OldPath = filepath.ToSlash(parts[1])
				entry.Path = filepath.ToSlash(parts[2])
			}
		default:
			entry.Path = filepath.ToSlash(parts[1])
		}
		if entry.Path == "" {
			continue
		}
		additions, deletions, binary, err := readPathNumstat(ctx, meta, baseRef, headRef, entry.Path, entry.OldPath, workingTree)
		if err == nil {
			entry.Additions = additions
			entry.Deletions = deletions
			entry.Binary = binary
		}
		entries = append(entries, entry)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})
	return entries, nil
}

func readPathNumstat(ctx context.Context, meta Meta, baseRef string, headRef string, path string, oldPath string, workingTree bool) (int, int, bool, error) {
	args := []string{"diff", "--numstat", "--find-renames", "--no-ext-diff"}
	if workingTree {
		args = append(args, baseRef)
	} else {
		args = append(args, baseRef+"..."+headRef)
	}
	paths := []string{path}
	if oldPath != "" && oldPath != path {
		paths = append(paths, oldPath)
	}
	args = append(args, "--")
	args = append(args, paths...)
	stdout, err := gitReadBytes(ctx, meta, args...)
	if err != nil {
		return 0, 0, false, err
	}
	lines := strings.Split(strings.TrimSpace(string(stdout)), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		binary := parts[0] == "-" || parts[1] == "-"
		additions := atoiDefault(parts[0], 0)
		deletions := atoiDefault(parts[1], 0)
		return additions, deletions, binary, nil
	}
	return 0, 0, false, nil
}

func readDiffPatch(ctx context.Context, meta Meta, baseRef string, headRef string, path string, oldPath string, workingTree bool) (string, error) {
	args := []string{"diff", "--find-renames", "--unified=3", "--no-ext-diff"}
	if workingTree {
		args = append(args, baseRef)
	} else {
		args = append(args, baseRef+"..."+headRef)
	}
	paths := []string{path}
	if oldPath != "" && oldPath != path {
		paths = append(paths, oldPath)
	}
	args = append(args, "--")
	args = append(args, paths...)
	stdout, err := gitReadBytes(ctx, meta, args...)
	if err != nil {
		return "", err
	}
	return util.RedactSecrets(string(stdout)), nil
}

func parseGitDiffHunks(patch string) []GitDiffHunk {
	lines := strings.Split(patch, "\n")
	hunks := []GitDiffHunk{}
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		match := gitDiffHunkHeaderPattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		hunk := GitDiffHunk{
			OldStart: atoiDefault(match[1], 0),
			OldLines: atoiDefault(defaultString(match[2], "1"), 1),
			NewStart: atoiDefault(match[3], 0),
			NewLines: atoiDefault(defaultString(match[4], "1"), 1),
		}

		oldLine := hunk.OldStart
		newLine := hunk.NewStart
		for j := i + 1; j < len(lines); j++ {
			next := lines[j]
			if gitDiffHunkHeaderPattern.MatchString(next) {
				i = j - 1
				break
			}
			if strings.HasPrefix(next, "--- ") || strings.HasPrefix(next, "+++ ") || strings.HasPrefix(next, "diff --git ") || strings.HasPrefix(next, "index ") {
				continue
			}
			if strings.HasPrefix(next, "\\ No newline at end of file") {
				continue
			}
			switch {
			case strings.HasPrefix(next, "+"):
				hunk.AddedLines = append(hunk.AddedLines, newLine)
				newLine++
			case strings.HasPrefix(next, "-"):
				hunk.DeletedLines = append(hunk.DeletedLines, oldLine)
				oldLine++
			default:
				oldLine++
				newLine++
			}
			if j == len(lines)-1 {
				i = j
			}
		}
		hunks = append(hunks, hunk)
	}
	return hunks
}

func readFileAtRef(ctx context.Context, meta Meta, ref string, path string, workingTree bool) (string, bool, error) {
	if workingTree {
		abs := filepath.Join(meta.RepoRoot, filepath.FromSlash(path))
		data, err := os.ReadFile(abs)
		if errors.Is(err, os.ErrNotExist) {
			return "", true, nil
		}
		if err != nil {
			return "", false, err
		}
		return sanitizeGitContent(string(data)), false, nil
	}
	if strings.TrimSpace(ref) == "" {
		return "", false, errors.New("ref is required")
	}
	stdout, err := gitReadBytes(ctx, meta, "show", fmt.Sprintf("%s:%s", ref, path))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "does not exist") || strings.Contains(strings.ToLower(err.Error()), "exists on disk") || strings.Contains(strings.ToLower(err.Error()), "path '") {
			return "", true, nil
		}
		return "", false, err
	}
	return sanitizeGitContent(string(stdout)), false, nil
}

func readBlameLines(ctx context.Context, meta Meta, ref string, path string, startLine int, endLine int) ([]GitBlameLine, error) {
	stdout, err := gitReadBytes(ctx, meta, "blame", "--line-porcelain", "-L", fmt.Sprintf("%d,%d", startLine, endLine), ref, "--", path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(stdout), "\n")
	out := []GitBlameLine{}
	var current GitBlameLine
	lineNo := startLine
	for _, raw := range lines {
		switch {
		case raw == "":
			continue
		case !strings.HasPrefix(raw, "\t") && len(strings.Fields(raw)) >= 3 && isLikelyHash(strings.Fields(raw)[0]):
			fields := strings.Fields(raw)
			current = GitBlameLine{Commit: fields[0], Line: lineNo}
		case strings.HasPrefix(raw, "author "):
			current.Author = strings.TrimPrefix(raw, "author ")
		case strings.HasPrefix(raw, "summary "):
			current.Summary = strings.TrimPrefix(raw, "summary ")
		case strings.HasPrefix(raw, "\t"):
			current.LineText = strings.TrimPrefix(raw, "\t")
			out = append(out, current)
			lineNo++
		}
	}
	return out, nil
}

func readLogRange(ctx context.Context, meta Meta, baseRef string, headRef string, path string, limit int) ([]GitLogEntry, error) {
	args := []string{"log", "--oneline", "--no-decorate", fmt.Sprintf("--max-count=%d", limit), baseRef + ".." + headRef}
	if path != "" {
		args = append(args, "--", path)
	}
	stdout, err := gitReadBytes(ctx, meta, args...)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(stdout)), "\n")
	out := make([]GitLogEntry, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		entry := GitLogEntry{Commit: parts[0]}
		if len(parts) > 1 {
			entry.Subject = parts[1]
		}
		out = append(out, entry)
	}
	return out, nil
}

func gitReadOneLine(ctx context.Context, meta Meta, args ...string) (string, error) {
	stdout, err := gitReadBytes(ctx, meta, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(stdout)), nil
}

func gitReadBytes(ctx context.Context, meta Meta, args ...string) ([]byte, error) {
	if meta.ToolTimeoutSeconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(meta.ToolTimeoutSeconds)*time.Second)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = meta.RepoRoot
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return nil, fmt.Errorf("git %s failed: %s", strings.Join(args, " "), message)
	}
	return stdout.Bytes(), nil
}

func sanitizeRepoRelativePath(repoRoot string, path string) (string, error) {
	cleaned := filepath.Clean(strings.TrimSpace(path))
	if cleaned == "" {
		return "", errors.New("path is required")
	}
	abs := cleaned
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(repoRoot, cleaned)
	}
	rel, err := filepath.Rel(repoRoot, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", errors.New("path must stay within repo root")
	}
	return filepath.ToSlash(rel), nil
}

func summarizeChangedFiles(files []GitChangedFile) string {
	lines := make([]string, 0, len(files))
	for _, file := range files {
		line := file.Status + "\t" + file.Path
		if file.OldPath != "" {
			line = file.Status + "\t" + file.OldPath + " -> " + file.Path
		}
		if file.Binary {
			line += " (binary)"
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func summarizeBlame(lines []GitBlameLine) string {
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		parts = append(parts, fmt.Sprintf("%d %s %s", line.Line, trimSHA(line.Commit), line.Summary))
	}
	return strings.Join(parts, "\n")
}

func summarizeLogEntries(entries []GitLogEntry) string {
	parts := make([]string, 0, len(entries))
	for _, entry := range entries {
		parts = append(parts, trimSHA(entry.Commit)+" "+entry.Subject)
	}
	return strings.Join(parts, "\n")
}

func sanitizeGitContent(content string) string {
	if strings.IndexByte(content, 0) >= 0 || !utf8.ValidString(content) {
		return ""
	}
	return util.RedactSecrets(content)
}

func normalizeGitStatus(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "M"
	}
	switch raw[0] {
	case 'A':
		return "A"
	case 'D':
		return "D"
	case 'R':
		return "R"
	case 'C':
		return "C"
	default:
		return "M"
	}
}

func defaultString(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func atoiDefault(value string, fallback int) int {
	value = strings.TrimSpace(value)
	if value == "" || value == "-" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func isLikelyHash(value string) bool {
	if len(value) < 7 {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func trimSHA(value string) string {
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}

func btoi(ok bool) int {
	if ok {
		return 1
	}
	return 0
}
