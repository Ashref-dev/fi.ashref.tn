package tools

import (
	"context"
	"encoding/json"
)

// Meta provides execution context to tools.
type Meta struct {
	RepoRoot           string
	UnsafeShell        bool
	ToolTimeoutSeconds int
	MaxBytes           int
	MaxResults         int
}

// Result is a structured tool execution result.
type Result struct {
	ToolName   string
	Payload    any
	Preview    string
	LineCount  int
	ByteCount  int
	Truncated  bool
	DurationMs int64
}

// Tool describes a callable tool.
type Tool interface {
	Name() string
	Description() string
	Schema() map[string]any
	Execute(ctx context.Context, input json.RawMessage, meta Meta) (Result, error)
}
