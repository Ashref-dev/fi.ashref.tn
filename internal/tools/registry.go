package tools

import (
	"sort"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
)

// Registry stores available tools.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry builds a registry from tools.
func NewRegistry(items ...Tool) *Registry {
	reg := &Registry{tools: map[string]Tool{}}
	for _, item := range items {
		reg.tools[item.Name()] = item
	}
	return reg
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	tool, ok := r.tools[name]
	return tool, ok
}

// Names returns sorted tool names.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// OpenAITools converts tool definitions to OpenAI tool schema.
func (r *Registry) OpenAITools() []openai.ChatCompletionToolUnionParam {
	var defs []openai.ChatCompletionToolUnionParam
	for _, tool := range r.tools {
		defs = append(defs, openai.ChatCompletionToolUnionParam{
			OfFunction: &openai.ChatCompletionFunctionToolParam{
				Function: shared.FunctionDefinitionParam{
					Name:        tool.Name(),
					Description: param.NewOpt(tool.Description()),
					Parameters:  tool.Schema(),
					Strict:      param.NewOpt(true),
				},
			},
		})
	}
	return defs
}
