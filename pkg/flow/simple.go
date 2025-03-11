package flow

import (
	"context"

	"github.com/prathyushnallamothu/commandforge/pkg/llm"
)

// SimpleFlow provides a basic flow implementation
type SimpleFlow struct {
	*BaseFlow
	LLMClient llm.Client
}

// NewSimpleFlow creates a new simple flow
func NewSimpleFlow(llmClient llm.Client, memory Memory) *SimpleFlow {
	return &SimpleFlow{
		BaseFlow:  NewBaseFlow("simple", "A simple flow that passes requests directly to an LLM", memory),
		LLMClient: llmClient,
	}
}

// Run processes a user request by passing it directly to the LLM
func (f *SimpleFlow) Run(ctx context.Context, request *FlowRequest) (*FlowResponse, error) {
	// Set the flow state to running
	f.setState(StateRunning)

	// Create a simple system prompt
	systemPrompt := "You are a helpful assistant. Answer the user's question concisely and accurately."

	// Create the conversation history
	conversation := []llm.Message{
		{
			Role:    "system",
			Content: systemPrompt,
		},
		{
			Role:    "user",
			Content: request.Input,
		},
	}

	// Send the request to the LLM
	chatRequest := &llm.ChatCompletionRequest{
		Messages: conversation,
	}

	chatResponse, err := f.LLMClient.ChatCompletion(ctx, chatRequest)
	if err != nil {
		// Set the flow state to error
		f.setState(StateError)
		return &FlowResponse{
			Output:  "",
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	// Extract the response content
	response := chatResponse.Choices[0].Message.Content

	// Set the flow state to complete
	f.setState(StateComplete)

	// Return the response
	return &FlowResponse{
		Output:  response,
		Success: true,
	}, nil
}
