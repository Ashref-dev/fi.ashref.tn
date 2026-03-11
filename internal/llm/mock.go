package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// MockClient is a deterministic client for tests and demos.
type MockClient struct {
	mu        sync.Mutex
	toolCalls int
	lastTool  string
}

// NewMockClient returns a simple mock.
func NewMockClient() *MockClient {
	return &MockClient{}
}

func (m *MockClient) Create(ctx context.Context, req Request) (Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Plan generation calls usually do not include tools.
	if len(req.Tools) == 0 {
		return Response{Content: "- Review repository context\n- Use grep to find signals\n- Summarize findings with citations"}, nil
	}

	m.toolCalls++
	if m.toolCalls == 1 {
		toolName := chooseMockToolName(req)
		m.lastTool = toolName
		args := mockToolArgs(toolName)
		return Response{ToolCalls: []ToolCall{{ID: "call_1", Name: toolName, Arguments: args}}}, nil
	}
	return Response{Content: mockFinalContent(m.lastTool)}, nil
}

func (m *MockClient) Stream(ctx context.Context, req Request, onDelta func(string)) (Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Keep stream behavior aligned with Create:
	// first model turn requests grep, second turn returns final text.
	if len(req.Tools) == 0 {
		resp := Response{Content: "- Review repository context\n- Use grep to find signals\n- Summarize findings with citations"}
		if onDelta != nil {
			onDelta(resp.Content)
		}
		return resp, nil
	}

	m.toolCalls++
	if m.toolCalls == 1 {
		toolName := chooseMockToolName(req)
		m.lastTool = toolName
		args := mockToolArgs(toolName)
		return Response{ToolCalls: []ToolCall{{ID: "call_1", Name: toolName, Arguments: args}}}, nil
	}

	content := mockFinalContent(m.lastTool)
	resp := Response{Content: content}
	if onDelta != nil {
		onDelta(content)
	}
	return resp, nil
}

func chooseMockToolName(req Request) string {
	toolBytes, _ := json.Marshal(req.Tools)
	toolText := strings.ToLower(string(toolBytes))
	if strings.Contains(toolText, "\"name\":\"grep\"") {
		return "grep"
	}
	if strings.Contains(toolText, "\"name\":\"list_tree\"") {
		return "list_tree"
	}
	return "grep"
}

func mockFinalContent(toolName string) string {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		toolName = "grep"
	}
	return fmt.Sprintf("Summary: Mock response based on tool results. [tool:%s]\nNext steps: Review the referenced files for details.", toolName)
}

func mockToolArgs(toolName string) json.RawMessage {
	switch toolName {
	case "list_tree":
		args, _ := json.Marshal(map[string]any{"path": ".", "max_depth": 3, "max_entries": 120})
		return args
	default:
		args, _ := json.Marshal(map[string]any{"pattern": "FICLI", "case_sensitive": false, "max_results": 20})
		return args
	}
}
