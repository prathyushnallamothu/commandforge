package llm

import (
	"context"
)

// Message represents a chat message
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall represents a tool call from the LLM
type ToolCall struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Function ToolCallFunction       `json:"function"`
	Args     map[string]interface{} `json:"args,omitempty"`
}

// ToolCallFunction represents the function details in a tool call
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolDefinition represents a tool that can be called by the LLM
type ToolDefinition struct {
	Type       string `json:"type"`
	Function   Function `json:"function"`
}

// Function represents a function definition for tool calling
type Function struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Parameters  Parameters `json:"parameters"`
}

// Parameters represents the parameters for a function
type Parameters struct {
	Type       string                 `json:"type"`
	Properties map[string]Property    `json:"properties"`
	Required   []string               `json:"required,omitempty"`
}

// Property represents a parameter property
type Property struct {
	Type        string      `json:"type"`
	Description string      `json:"description"`
	Enum        []string    `json:"enum,omitempty"`
	Items       *Property   `json:"items,omitempty"`
}

// ChatCompletionRequest represents a request for chat completion
type ChatCompletionRequest struct {
	Model       string           `json:"model"`
	Messages    []Message        `json:"messages"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
	Temperature float64          `json:"temperature,omitempty"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
}

// ChatCompletionResponse represents a response from chat completion
type ChatCompletionResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage `json:"usage"`
}

// Choice represents a completion choice
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// Usage represents token usage information
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Client defines the interface for LLM clients
type Client interface {
	// ChatCompletion generates a chat completion
	ChatCompletion(ctx context.Context, request *ChatCompletionRequest) (*ChatCompletionResponse, error)
	
	// GetModelName returns the name of the model being used
	GetModelName() string
	
	// GetProvider returns the name of the LLM provider
	GetProvider() string
}
