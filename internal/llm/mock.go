package llm

import (
	"context"
	"encoding/json"
	"sync"
)

// MockClient is a deterministic client for tests and demos.
type MockClient struct {
	mu        sync.Mutex
	toolCalls int
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
		args, _ := json.Marshal(map[string]any{"pattern": "FICLI", "case_sensitive": false, "max_results": 20})
		return Response{ToolCalls: []ToolCall{{ID: "call_1", Name: "grep", Arguments: args}}}, nil
	}
	return Response{Content: "Summary: Mock response based on tool results. [tool:grep]\nNext steps: Review the referenced files for details."}, nil
}

func (m *MockClient) Stream(ctx context.Context, req Request, onDelta func(string)) (Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	content := "Summary: Mock response based on tool results. [tool:grep]\nNext steps: Review the referenced files for details."
	resp := Response{Content: content}
	if onDelta != nil {
		onDelta(resp.Content)
	}
	return resp, nil
}
