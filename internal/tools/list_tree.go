package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"fi-cli/internal/repo"
)

const (
	defaultListTreeMaxDepth   = 3
	defaultListTreeMaxEntries = 200
)

type ListTreeTool struct{}

func NewListTreeTool() *ListTreeTool {
	return &ListTreeTool{}
}

func (t *ListTreeTool) Name() string { return "list_tree" }

func (t *ListTreeTool) Description() string {
	return "List repository folders/files as a depth-limited tree with safe ignore rules."
}

func (t *ListTreeTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":        map[string]any{"type": "string"},
			"max_depth":   map[string]any{"type": "integer", "minimum": 0, "maximum": 10},
			"max_entries": map[string]any{"type": "integer", "minimum": 1, "maximum": 2000},
		},
		"additionalProperties": false,
	}
}

type listTreeInput struct {
	Path       string `json:"path"`
	MaxDepth   int    `json:"max_depth"`
	MaxEntries int    `json:"max_entries"`
}

type listTreeOutput struct {
	Path       string   `json:"path"`
	Lines      []string `json:"lines"`
	Truncated  bool     `json:"truncated"`
	MaxDepth   int      `json:"max_depth"`
	MaxEntries int      `json:"max_entries"`
	DurationMs int64    `json:"duration_ms"`
}

// ListTreeOutputAlias exposes list_tree output for typed consumers without renaming the internal type.
type ListTreeOutputAlias = listTreeOutput

type treeEntry struct {
	name  string
	abs   string
	rel   string
	isDir bool
}

type listTreeState struct {
	lines     []string
	truncated bool
	entries   int
}

func (t *ListTreeTool) Execute(ctx context.Context, input json.RawMessage, meta Meta) (Result, error) {
	if meta.ToolTimeoutSeconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(meta.ToolTimeoutSeconds)*time.Second)
		defer cancel()
	}

	var args listTreeInput
	if len(input) > 0 {
		if err := json.Unmarshal(input, &args); err != nil {
			return Result{}, err
		}
	}

	targetPath := strings.TrimSpace(args.Path)
	if targetPath == "" {
		targetPath = "."
	}
	maxDepth := args.MaxDepth
	if maxDepth <= 0 {
		maxDepth = defaultListTreeMaxDepth
	}
	if maxDepth > 10 {
		maxDepth = 10
	}
	maxEntries := args.MaxEntries
	if maxEntries <= 0 {
		maxEntries = defaultListTreeMaxEntries
	}
	if maxEntries > 2000 {
		maxEntries = 2000
	}

	absTarget, relTarget, err := resolveTreePath(meta.RepoRoot, targetPath)
	if err != nil {
		return Result{}, err
	}

	start := time.Now()
	info, err := os.Stat(absTarget)
	if err != nil {
		return Result{}, err
	}

	state := &listTreeState{}
	rootLabel := relTarget
	if info.IsDir() {
		if relTarget == "." {
			rootLabel = "./"
		} else {
			rootLabel += "/"
		}
	}
	state.lines = append(state.lines, rootLabel)

	if info.IsDir() {
		if err := t.walk(ctx, absTarget, relTarget, 0, "", maxDepth, maxEntries, state); err != nil {
			return Result{}, err
		}
	}

	output := listTreeOutput{
		Path:       relTarget,
		Lines:      state.lines,
		Truncated:  state.truncated,
		MaxDepth:   maxDepth,
		MaxEntries: maxEntries,
		DurationMs: time.Since(start).Milliseconds(),
	}
	preview := strings.Join(state.lines, "\n")
	payloadBytes, _ := json.Marshal(output)
	if meta.MaxBytes > 0 && len(payloadBytes) > meta.MaxBytes {
		output.Truncated = true
		for len(output.Lines) > 1 {
			output.Lines = output.Lines[:len(output.Lines)-1]
			output.Lines = append(output.Lines, "...")
			payloadBytes, _ = json.Marshal(output)
			if len(payloadBytes) <= meta.MaxBytes {
				break
			}
		}
		preview = strings.Join(output.Lines, "\n")
	}

	return Result{
		ToolName:   t.Name(),
		Payload:    output,
		Preview:    preview,
		LineCount:  len(output.Lines),
		ByteCount:  len(payloadBytes),
		Truncated:  output.Truncated,
		DurationMs: output.DurationMs,
	}, nil
}

func (t *ListTreeTool) walk(ctx context.Context, absPath string, relPath string, depth int, prefix string, maxDepth int, maxEntries int, state *listTreeState) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return nil
	}
	filtered := filterTreeEntries(entries, absPath, relPath)
	if len(filtered) == 0 {
		return nil
	}

	if depth >= maxDepth {
		state.lines = append(state.lines, prefix+"...")
		state.truncated = true
		return nil
	}

	for i, entry := range filtered {
		if err := ctx.Err(); err != nil {
			return err
		}
		if state.entries >= maxEntries {
			state.lines = append(state.lines, prefix+"...")
			state.truncated = true
			return nil
		}
		isLast := i == len(filtered)-1
		connector := "├── "
		nextPrefix := prefix + "│   "
		if isLast {
			connector = "└── "
			nextPrefix = prefix + "    "
		}
		label := entry.name
		if entry.isDir {
			label += "/"
		}
		state.lines = append(state.lines, prefix+connector+label)
		state.entries++
		if entry.isDir {
			if err := t.walk(ctx, entry.abs, entry.rel, depth+1, nextPrefix, maxDepth, maxEntries, state); err != nil {
				return err
			}
		}
	}
	return nil
}

func filterTreeEntries(entries []os.DirEntry, absDir string, relDir string) []treeEntry {
	ignoreDirs := map[string]struct{}{
		".git": {}, "node_modules": {}, "dist": {}, "build": {}, "coverage": {}, ".next": {}, ".turbo": {},
	}
	filtered := make([]treeEntry, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if name == "" {
			continue
		}
		if entry.IsDir() {
			if _, ok := ignoreDirs[name]; ok {
				continue
			}
		}
		abs := filepath.Join(absDir, name)
		if repo.IsDenylisted(abs) {
			continue
		}
		rel := name
		if relDir != "." && relDir != "" {
			rel = filepath.Join(relDir, name)
		}
		filtered = append(filtered, treeEntry{name: name, abs: abs, rel: rel, isDir: entry.IsDir()})
	}

	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].isDir != filtered[j].isDir {
			return filtered[i].isDir
		}
		return strings.ToLower(filtered[i].name) < strings.ToLower(filtered[j].name)
	})
	return filtered
}

func resolveTreePath(repoRoot string, requested string) (string, string, error) {
	cleaned := filepath.Clean(requested)
	if cleaned == "" {
		cleaned = "."
	}
	abs := cleaned
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(repoRoot, cleaned)
	}
	rel, err := filepath.Rel(repoRoot, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", "", errors.New("path must stay within repo root")
	}
	if rel == "" {
		rel = "."
	}
	return abs, rel, nil
}
