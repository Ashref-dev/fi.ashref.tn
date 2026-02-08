package llm

import (
	"context"
	"encoding/json"

	"github.com/openai/openai-go/v3"
)

// ToolCall represents a model tool call.
type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

// Response represents a model response.
type Response struct {
	Content   string
	ToolCalls []ToolCall
}

// Request is a simplified chat completion request.
type Request struct {
	Model      string
	Messages   []openai.ChatCompletionMessageParamUnion
	Tools      []openai.ChatCompletionToolUnionParam
	ToolChoice openai.ChatCompletionToolChoiceOptionUnionParam
}

// Client is an LLM client interface.
type Client interface {
	Create(ctx context.Context, req Request) (Response, error)
	Stream(ctx context.Context, req Request, onDelta func(string)) (Response, error)
}
