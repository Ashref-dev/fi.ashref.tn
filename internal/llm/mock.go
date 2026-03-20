package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
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

	if isMockReviewRequest(req) {
		return Response{Content: mockReviewContent(req)}, nil
	}

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

	if isMockReviewRequest(req) {
		content := mockReviewContent(req)
		resp := Response{Content: content}
		if onDelta != nil {
			onDelta(content)
		}
		return resp, nil
	}

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

var reviewPathPattern = regexp.MustCompile(`(?is)changed file:\\npath:\s*([^\\\n"]+)`)

func isMockReviewRequest(req Request) bool {
	text := strings.ToLower(mockRequestText(req))
	return strings.Contains(text, "return the json review object now") ||
		strings.Contains(text, "review this changed file in repository context")
}

func mockReviewContent(req Request) string {
	raw := mockRequestText(req)
	lower := strings.ToLower(raw)
	filePath := ""
	if match := reviewPathPattern.FindStringSubmatch(raw); len(match) > 1 {
		filePath = strings.TrimSpace(match[1])
	}

	makeFinding := func(severity string, category string, title string, body string, suggestedFix string) []map[string]any {
		return []map[string]any{{
			"severity":      severity,
			"category":      category,
			"confidence":    0.92,
			"title":         title,
			"body":          body,
			"file":          filePath,
			"line_start":    1,
			"line_end":      1,
			"evidence":      "diff hunk",
			"suggested_fix": suggestedFix,
		}}
	}

	response := map[string]any{
		"summary":     "No material issues found in this file.",
		"strengths":   []string{"Change scope is focused and readable."},
		"uncertainty": []string{},
		"findings":    []map[string]any{},
	}

	switch {
	case strings.Contains(lower, "review:blocker"), strings.Contains(lower, "bugblocker"), strings.Contains(lower, "panic("):
		response["summary"] = "Found a blocker in the changed file."
		response["findings"] = makeFinding(
			"blocker",
			"correctness",
			"Potential runtime failure introduced in changed code",
			"The diff appears to introduce behavior that can fail at runtime and should be fixed before merge.",
			"Remove the failing path or guard it with safe error handling and add a regression test.",
		)
	case strings.Contains(lower, "review:high"):
		response["summary"] = "Found a high-severity issue in the changed file."
		response["findings"] = makeFinding(
			"high",
			"correctness",
			"High-risk behavior change",
			"The changed code looks likely to regress existing behavior under common inputs.",
			"Restore the prior guard conditions or add the missing validation before this path executes.",
		)
	case strings.Contains(lower, "review:medium"), strings.Contains(lower, "missing test"):
		response["summary"] = "Found a medium-severity issue in the changed file."
		response["findings"] = makeFinding(
			"medium",
			"tests",
			"Missing regression coverage for changed behavior",
			"The diff changes behavior without corresponding tests, which increases regression risk.",
			"Add a focused regression test covering the new and existing expected behavior.",
		)
	case strings.Contains(lower, "review:low"):
		response["summary"] = "Found a low-severity issue in the changed file."
		response["findings"] = makeFinding(
			"low",
			"performance",
			"Minor inefficiency in changed code",
			"The change introduces a minor inefficiency that is unlikely to block merge.",
			"Consider the simpler or more direct implementation if this path is hot.",
		)
	}

	payload, _ := json.Marshal(response)
	return string(payload)
}

func mockRequestText(req Request) string {
	data, _ := json.Marshal(req.Messages)
	return string(data)
}
