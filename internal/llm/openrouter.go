package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
)

// OpenRouterClient implements Client using OpenRouter via OpenAI-compatible API.
type OpenRouterClient struct {
	client openai.Client
}

// NewOpenRouterClient constructs a client with base URL and headers.
func NewOpenRouterClient(apiKey, baseURL, referer, title string) *OpenRouterClient {
	opts := []option.RequestOption{option.WithAPIKey(apiKey)}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	if referer != "" {
		opts = append(opts, option.WithHeader("HTTP-Referer", referer))
	}
	if title != "" {
		opts = append(opts, option.WithHeader("X-Title", title))
	}
	client := openai.NewClient(opts...)
	return &OpenRouterClient{client: client}
}

func (c *OpenRouterClient) Create(ctx context.Context, req Request) (Response, error) {
	params := openai.ChatCompletionNewParams{
		Model:       shared.ChatModel(req.Model),
		Messages:    req.Messages,
		Tools:       req.Tools,
		ToolChoice:  req.ToolChoice,
		Temperature: param.NewOpt(0.2),
	}
	resp, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return Response{}, err
	}
	return parseChatCompletion(resp)
}

func (c *OpenRouterClient) Stream(ctx context.Context, req Request, onDelta func(string)) (Response, error) {
	params := openai.ChatCompletionNewParams{
		Model:       shared.ChatModel(req.Model),
		Messages:    req.Messages,
		Tools:       req.Tools,
		ToolChoice:  req.ToolChoice,
		Temperature: param.NewOpt(0.2),
	}
	stream := c.client.Chat.Completions.NewStreaming(ctx, params)
	var builder strings.Builder
	var accumulator openai.ChatCompletionAccumulator
	sawToolCallDelta := false
	for stream.Next() {
		chunk := stream.Current()
		if !accumulator.AddChunk(chunk) {
			return Response{}, fmt.Errorf("failed to accumulate streaming response")
		}
		for _, choice := range chunk.Choices {
			if len(choice.Delta.ToolCalls) > 0 {
				sawToolCallDelta = true
			}
			delta := choice.Delta.Content
			if delta != "" && !sawToolCallDelta {
				builder.WriteString(delta)
				if onDelta != nil {
					onDelta(delta)
				}
			}
		}
	}
	if err := stream.Err(); err != nil {
		return Response{}, err
	}
	response := Response{Content: builder.String()}
	if len(accumulator.Choices) > 0 {
		response.Content = accumulator.Choices[0].Message.Content
		for _, toolCall := range accumulator.Choices[0].Message.ToolCalls {
			if toolCall.Type != "function" {
				continue
			}
			response.ToolCalls = append(response.ToolCalls, ToolCall{
				ID:        toolCall.ID,
				Name:      toolCall.Function.Name,
				Arguments: json.RawMessage(toolCall.Function.Arguments),
			})
		}
	}
	return response, nil
}

func parseChatCompletion(resp *openai.ChatCompletion) (Response, error) {
	if resp == nil || len(resp.Choices) == 0 {
		return Response{}, fmt.Errorf("empty response")
	}
	msg := resp.Choices[0].Message
	response := Response{Content: msg.Content}
	for _, toolCall := range msg.ToolCalls {
		if toolCall.Type != "function" {
			continue
		}
		fn := toolCall.AsFunction()
		args := json.RawMessage(fn.Function.Arguments)
		response.ToolCalls = append(response.ToolCalls, ToolCall{
			ID:        fn.ID,
			Name:      fn.Function.Name,
			Arguments: args,
		})
	}
	return response, nil
}
