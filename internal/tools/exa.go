package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
)

type ExaTool struct {
	apiKey string
	client *retryablehttp.Client
}

// NewExaTool constructs an Exa search tool.
func NewExaTool(apiKey string) *ExaTool {
	client := retryablehttp.NewClient()
	client.RetryMax = 2
	client.Logger = nil
	return &ExaTool{apiKey: apiKey, client: client}
}

func (e *ExaTool) Name() string { return "exa_search" }

func (e *ExaTool) Description() string {
	return "Search the web via Exa and return titles, URLs, and snippets."
}

func (e *ExaTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query":        map[string]any{"type": "string"},
			"num_results":  map[string]any{"type": "integer", "minimum": 1, "maximum": 10},
			"include_text": map[string]any{"type": "boolean"},
		},
		"required":             []string{"query"},
		"additionalProperties": false,
	}
}

type exaInput struct {
	Query       string `json:"query"`
	NumResults  int    `json:"num_results"`
	IncludeText *bool  `json:"include_text"`
}

type exaResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

type exaOutput struct {
	Results    []exaResult `json:"results"`
	DurationMs int64       `json:"duration_ms"`
	Truncated  bool        `json:"truncated"`
}

func (e *ExaTool) Execute(ctx context.Context, input json.RawMessage, meta Meta) (Result, error) {
	if strings.TrimSpace(e.apiKey) == "" {
		return Result{}, errors.New("EXA_API_KEY is missing")
	}

	var args exaInput
	if err := json.Unmarshal(input, &args); err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(args.Query) == "" {
		return Result{}, errors.New("query is required")
	}
	if args.NumResults <= 0 {
		args.NumResults = 5
	}
	if args.NumResults > 10 {
		args.NumResults = 10
	}
	includeText := true
	if args.IncludeText != nil {
		includeText = *args.IncludeText
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(meta.ToolTimeoutSeconds)*time.Second)
	defer cancel()

	payload := map[string]any{
		"query":      args.Query,
		"numResults": args.NumResults,
	}
	if includeText {
		payload["contents"] = map[string]any{"text": true}
	}

	body, _ := json.Marshal(payload)
	request, err := retryablehttp.NewRequestWithContext(ctx, http.MethodPost, "https://api.exa.ai/search", bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	request.Header.Set("x-api-key", e.apiKey)
	request.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(request)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return Result{}, fmt.Errorf("exa search failed: %s", string(b))
	}

	var raw struct {
		Results []struct {
			Title string `json:"title"`
			URL   string `json:"url"`
			Text  string `json:"text"`
		} `json:"results"`
	}
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&raw); err != nil {
		return Result{}, err
	}

	results := make([]exaResult, 0, len(raw.Results))
	for _, item := range raw.Results {
		results = append(results, exaResult{Title: item.Title, URL: item.URL, Snippet: item.Text})
	}

	truncated, byteCount := fitExaResults(&results, meta.MaxBytes)
	output := exaOutput{Results: results, DurationMs: time.Since(start).Milliseconds(), Truncated: truncated}
	preview := buildPreview(results)
	lineCount := strings.Count(preview, "\n") + 1
	return Result{ToolName: e.Name(), Payload: output, Preview: preview, LineCount: lineCount, ByteCount: byteCount, Truncated: truncated, DurationMs: output.DurationMs}, nil
}

func fitExaResults(results *[]exaResult, maxBytes int) (bool, int) {
	if maxBytes <= 0 {
		return false, 0
	}
	truncated := false
	snippetLimit := 1200
	for {
		if snippetLimit < 200 {
			break
		}
		for i := range *results {
			snippet := (*results)[i].Snippet
			if len(snippet) > snippetLimit {
				(*results)[i].Snippet = snippet[:snippetLimit]
				truncated = true
			}
		}
		payload := exaOutput{Results: *results}
		data, _ := json.Marshal(payload)
		if len(data) <= maxBytes {
			return truncated, len(data)
		}
		snippetLimit /= 2
	}
	for len(*results) > 1 {
		*results = (*results)[:len(*results)-1]
		truncated = true
		payload := exaOutput{Results: *results}
		data, _ := json.Marshal(payload)
		if len(data) <= maxBytes {
			return truncated, len(data)
		}
	}
	payload := exaOutput{Results: *results}
	data, _ := json.Marshal(payload)
	return truncated, len(data)
}

func buildPreview(results []exaResult) string {
	var b strings.Builder
	max := 3
	if len(results) < max {
		max = len(results)
	}
	for i := 0; i < max; i++ {
		item := results[i]
		b.WriteString(fmt.Sprintf("%s - %s\n", item.Title, item.URL))
		if item.Snippet != "" {
			b.WriteString(item.Snippet)
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String())
}
