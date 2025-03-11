package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/prathyushnallamothu/commandforge/pkg/llm"
	"github.com/prathyushnallamothu/commandforge/pkg/tools"
)

// ToolCallAgent implements a specialized agent focused on structured tool calling
type ToolCallAgent struct {
	*BaseAgent
	LLMClient           llm.Client
	ToolHandler         *llm.ToolCallingHandler
	ToolCollection      *tools.ToolCollection
	ConversationHistory []llm.Message
	MaxHistorySize      int
	SystemPrompt        string
	MaxIterations       int
	StreamingEnabled    bool
}

// NewToolCallAgent creates a new tool call agent
func NewToolCallAgent(name string, llmClient llm.Client, memory Memory) *ToolCallAgent {
	// Create tool collection
	toolCollection := tools.NewToolCollection()

	// Create tool handler
	toolHandler := llm.NewToolCallingHandler(toolCollection)

	// Create base agent
	baseAgent := NewBaseAgent(
		name,
		"A specialized agent for structured tool calling and execution",
		memory,
	)

	// Create ToolCall agent
	agent := &ToolCallAgent{
		BaseAgent:           baseAgent,
		LLMClient:           llmClient,
		ToolHandler:         toolHandler,
		ToolCollection:      toolCollection,
		ConversationHistory: make([]llm.Message, 0),
		MaxHistorySize:      50,
		SystemPrompt:        defaultToolCallSystemPrompt,
		MaxIterations:       10, // Prevent infinite loops
		StreamingEnabled:    true,
	}

	return agent
}

// defaultToolCallSystemPrompt is the default system prompt for the ToolCall agent
const defaultToolCallSystemPrompt = `You are CommandForge, an autonomous AI agent designed to help users execute commands and perform tasks.

You have access to a set of powerful tools that allow you to interact with the system. When solving problems:

1. ALWAYS use the most appropriate tool for the task
2. CHAIN multiple tool calls together to solve complex problems
3. PROVIDE clear explanations of your actions and findings

You have access to various tools including:
- Running bash commands with real-time output streaming
- Executing Python code
- Managing files
- Searching the web
- Browsing web pages

When executing commands, you'll receive real-time streaming output, allowing you to monitor progress and make decisions based on intermediate results.

Be thorough, proactive, and always prioritize the user's goals. If a tool execution fails, try to understand why and either fix the issue or suggest alternatives.`

// WithSystemPrompt sets a custom system prompt
func (a *ToolCallAgent) WithSystemPrompt(prompt string) *ToolCallAgent {
	a.SystemPrompt = prompt
	return a
}

// WithMaxHistorySize sets the maximum conversation history size
func (a *ToolCallAgent) WithMaxHistorySize(size int) *ToolCallAgent {
	a.MaxHistorySize = size
	return a
}

// WithMaxIterations sets the maximum number of reasoning iterations
func (a *ToolCallAgent) WithMaxIterations(iterations int) *ToolCallAgent {
	a.MaxIterations = iterations
	return a
}

// WithStreamingEnabled enables or disables streaming output for commands
func (a *ToolCallAgent) WithStreamingEnabled(enabled bool) *ToolCallAgent {
	a.StreamingEnabled = enabled
	return a
}

// AddTool adds a tool to the agent
func (a *ToolCallAgent) AddTool(tool tools.Tool) error {
	return a.ToolCollection.AddTool(tool)
}

// Initialize sets up the agent
func (a *ToolCallAgent) Initialize(ctx context.Context) error {
	// Initialize the base agent
	if err := a.BaseAgent.Initialize(ctx); err != nil {
		return err
	}

	// Add the system message to the conversation history
	a.ConversationHistory = append(a.ConversationHistory, llm.Message{
		Role:    "system",
		Content: a.SystemPrompt,
	})

	return nil
}

// Run processes a user request
func (a *ToolCallAgent) Run(ctx context.Context, request *Request) (*Response, error) {
	// Set the agent state to running
	a.setState(StateRunning)

	// Add the user message to the conversation history
	a.ConversationHistory = append(a.ConversationHistory, llm.Message{
		Role:    "user",
		Content: request.Input,
	})

	// Process the request
	output, err := a.processRequest(ctx, request)
	if err != nil {
		// Set the agent state to error
		a.setState(StateError)
		return &Response{
			Output:  "",
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	// Set the agent state back to idle
	a.setState(StateIdle)

	// Return the response
	return &Response{
		Output:  output,
		Success: true,
	}, nil
}

// processRequest processes a user request using structured tool calling
func (a *ToolCallAgent) processRequest(ctx context.Context, request *Request) (string, error) {
	// Initialize iteration counter to prevent infinite loops
	iterationCount := 0

	// Start the tool calling loop
	for iterationCount < a.MaxIterations {
		// Increment the iteration counter
		iterationCount++

		// Create a chat completion request
		chatRequest := &llm.ChatCompletionRequest{
			Messages:    a.ConversationHistory,
			Tools:       a.ToolHandler.GenerateToolDefinitions(),
			Temperature: 0.7,
		}

		// Send the request to the LLM
		completion, err := a.LLMClient.ChatCompletion(ctx, chatRequest)
		if err != nil {
			return "", fmt.Errorf("failed to get chat completion: %w", err)
		}

		// Check if there are any choices
		if len(completion.Choices) == 0 {
			return "", fmt.Errorf("no completion choices returned")
		}

		// Get the assistant's message
		message := completion.Choices[0].Message

		// Add the message to the conversation history
		a.ConversationHistory = append(a.ConversationHistory, message)

		// Check for tool calls
		toolCalls, err := llm.ParseToolCalls(message)
		if err != nil || len(toolCalls) == 0 {
			// No tool calls, just return the message content
			return message.Content, nil
		}

		// Process tool calls with special handling for streaming commands
		toolResults, err := a.processToolCalls(ctx, toolCalls)
		if err != nil {
			return "", fmt.Errorf("failed to process tool calls: %w", err)
		}

		// Add tool results to conversation history
		a.ConversationHistory = append(a.ConversationHistory, toolResults...)

		// Check if we need to trim the conversation history
		a.trimConversationHistory()
	}

	// If we've reached the maximum number of iterations, return a timeout error
	return "", fmt.Errorf("reached maximum number of iterations (%d) without completing the task", a.MaxIterations)
}

// processToolCalls processes tool calls with special handling for streaming commands
func (a *ToolCallAgent) processToolCalls(ctx context.Context, toolCalls []llm.ToolCall) ([]llm.Message, error) {
	if len(toolCalls) == 0 {
		return nil, nil
	}

	// Process each tool call and collect results
	resultMessages := make([]llm.Message, 0, len(toolCalls))

	for _, tc := range toolCalls {
		// Check if this is a command execution tool that supports streaming
		if a.StreamingEnabled && isStreamingCommandTool(tc.Function.Name) {
			// Handle streaming command execution
			resultMsg, err := a.handleStreamingCommand(ctx, tc)
			if err != nil {
				// Create an error message
				resultMsg = llm.Message{
					Role: "tool",
					Content: fmt.Sprintf(
						"Tool execution failed: %s\nError: %v",
						tc.Function.Name,
						err,
					),
				}
			}
			resultMessages = append(resultMessages, resultMsg)
		} else {
			// Standard tool execution
			result, err := a.ToolHandler.ExecuteToolByName(ctx, tc.Function.Name, tc.Args)

			// Create a result message
			message := llm.Message{
				Role:    "tool",
				Content: "",
			}

			// Format the content based on success or failure
			if err != nil {
				// Tool execution failed
				message.Content = fmt.Sprintf(
					"Tool execution failed: %s\nError: %v",
					tc.Function.Name,
					err,
				)
			} else {
				// Tool execution succeeded
				resultJSON, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					resultJSON = []byte(fmt.Sprintf("%v", result))
				}

				message.Content = string(resultJSON)
			}

			resultMessages = append(resultMessages, message)
		}
	}

	return resultMessages, nil
}

// handleStreamingCommand handles streaming command execution
func (a *ToolCallAgent) handleStreamingCommand(ctx context.Context, tc llm.ToolCall) (llm.Message, error) {
	// Extract command parameters
	cmd, ok := tc.Args["command"].(string)
	if !ok || cmd == "" {
		return llm.Message{}, fmt.Errorf("command parameter is required and must be a string")
	}

	// Extract working directory if provided
	cwd, _ := tc.Args["cwd"].(string)
	if cwd == "" {
		cwd = "."
	}

	// Create a command tool
	commandTool, err := a.ToolCollection.GetTool(tc.Function.Name)
	if err != nil {
		return llm.Message{}, fmt.Errorf("command tool not found: %s", tc.Function.Name)
	}

	// Execute the command with streaming
	result, err := commandTool.Execute(ctx, tc.Args)
	if err != nil {
		return llm.Message{}, fmt.Errorf("failed to execute command: %w", err)
	}

	// Format the result as a message
	resultJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return llm.Message{
			Role:    "tool",
			Content: fmt.Sprintf("%v", result),
		}, nil
	}

	return llm.Message{
		Role:    "tool",
		Content: string(resultJSON),
	}, nil
}

// isStreamingCommandTool checks if a tool supports streaming
func isStreamingCommandTool(toolName string) bool {
	// List of tools that support streaming
	streamingTools := map[string]bool{
		"run_command":        true,
		"run_shell_command":  true,
		"execute_command":    true,
		"execute_background": true,
	}

	return streamingTools[toolName]
}

// trimConversationHistory trims the conversation history to the maximum size
func (a *ToolCallAgent) trimConversationHistory() {
	// If the history is smaller than the maximum size, do nothing
	if len(a.ConversationHistory) <= a.MaxHistorySize {
		return
	}

	// Keep the system message and the most recent messages
	newHistory := make([]llm.Message, 0, a.MaxHistorySize)

	// Always keep the system message
	newHistory = append(newHistory, a.ConversationHistory[0])

	// Keep the most recent messages
	start := len(a.ConversationHistory) - a.MaxHistorySize + 1
	newHistory = append(newHistory, a.ConversationHistory[start:]...)

	a.ConversationHistory = newHistory
}

// ExecuteTool executes a tool by name
func (a *ToolCallAgent) ExecuteTool(ctx context.Context, name string, params map[string]interface{}) (interface{}, error) {
	return a.ToolHandler.ExecuteToolByName(ctx, name, params)
}

// GetConversationHistory returns the conversation history
func (a *ToolCallAgent) GetConversationHistory() []llm.Message {
	return a.ConversationHistory
}

// SaveConversation saves the conversation history to memory
func (a *ToolCallAgent) SaveConversation(ctx context.Context, key string) error {
	// Convert the conversation history to JSON
	conversationJSON, err := json.Marshal(a.ConversationHistory)
	if err != nil {
		return fmt.Errorf("failed to marshal conversation history: %w", err)
	}

	// Save the conversation history to memory
	return a.Memory.Save(ctx, key, string(conversationJSON))
}

// LoadConversation loads conversation history from memory
func (a *ToolCallAgent) LoadConversation(ctx context.Context, key string) error {
	// Get the conversation history from memory
	value, err := a.Memory.Load(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to get conversation history from memory: %w", err)
	}

	// Convert the value to a string
	conversationJSON, ok := value.(string)
	if !ok {
		return fmt.Errorf("conversation history is not a string")
	}

	// Parse the conversation history
	var conversationHistory []llm.Message
	if err := json.Unmarshal([]byte(conversationJSON), &conversationHistory); err != nil {
		return fmt.Errorf("failed to unmarshal conversation history: %w", err)
	}

	// Set the conversation history
	a.ConversationHistory = conversationHistory

	return nil
}
