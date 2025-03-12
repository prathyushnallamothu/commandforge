package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/prathyushnallamothu/commandforge/pkg/llm"
	"github.com/prathyushnallamothu/commandforge/pkg/tools"
)

// CommandForgeAgent is the main agent for CommandForge
// It combines the ReAct and ToolCall patterns for robust reasoning and tool execution
type CommandForgeAgent struct {
	*BaseAgent
	LLMClient           llm.Client
	ToolHandler         *llm.ToolCallingHandler
	ToolCollection      *tools.ToolCollection
	ConversationHistory []llm.Message
	MaxHistorySize      int
	SystemPrompt        string
	MaxIterations       int
	StreamingEnabled    bool
	ReActEnabled        bool
}

// NewCommandForgeAgent creates a new CommandForge agent
func NewCommandForgeAgent(name string, llmClient llm.Client, memory Memory) *CommandForgeAgent {
	// Create tool collection
	toolCollection := tools.NewToolCollection()

	// Create tool handler
	toolHandler := llm.NewToolCallingHandler(toolCollection)

	// Create base agent
	baseAgent := NewBaseAgent(
		name,
		"The main CommandForge agent combining ReAct and ToolCall patterns for robust reasoning and tool execution",
		memory,
	)

	// Create CommandForge agent
	agent := &CommandForgeAgent{
		BaseAgent:           baseAgent,
		LLMClient:           llmClient,
		ToolHandler:         toolHandler,
		ToolCollection:      toolCollection,
		ConversationHistory: make([]llm.Message, 0),
		MaxHistorySize:      50,
		SystemPrompt:        defaultCommandForgeSystemPrompt,
		MaxIterations:       10, // Prevent infinite loops
		StreamingEnabled:    true,
		ReActEnabled:        true,
	}

	return agent
}

// defaultCommandForgeSystemPrompt is the default system prompt for the CommandForge agent
const defaultCommandForgeSystemPrompt = `You are CommandForge, an autonomous AI agent designed to help users execute commands and perform tasks.

You combine the ReAct (Reasoning and Acting) pattern with structured tool calling to solve problems effectively:

1. THINK: Carefully analyze the problem and plan your approach
2. ACT: Execute relevant tools to gather information or perform actions
3. OBSERVE: Review the results of your actions
4. REPEAT: Continue the cycle until the task is complete

When reasoning with the ReAct pattern, structure your thinking as follows:
Thought: <your step-by-step reasoning process>
Action: <the tool you want to use>
Action Input: <the parameters for the tool>
Observation: <the result from executing the tool>

Finish with:
Thought: I now have the information needed to answer the user's question.
Final Answer: <your comprehensive response to the user>

You have access to various tools that allow you to interact with the system, including:
- Running bash commands with real-time output streaming
- Executing Python code
- Managing files
- Searching the web
- Browsing web pages

Be thorough in your reasoning, proactive in your actions, and clear in your final answers.`

// WithSystemPrompt sets a custom system prompt
func (a *CommandForgeAgent) WithSystemPrompt(prompt string) *CommandForgeAgent {
	a.SystemPrompt = prompt
	return a
}

// WithMaxHistorySize sets the maximum conversation history size
func (a *CommandForgeAgent) WithMaxHistorySize(size int) *CommandForgeAgent {
	a.MaxHistorySize = size
	return a
}

// WithMaxIterations sets the maximum number of reasoning iterations
func (a *CommandForgeAgent) WithMaxIterations(iterations int) *CommandForgeAgent {
	a.MaxIterations = iterations
	return a
}

// WithStreamingEnabled enables or disables streaming output for commands
func (a *CommandForgeAgent) WithStreamingEnabled(enabled bool) *CommandForgeAgent {
	a.StreamingEnabled = enabled
	return a
}

// WithReActEnabled enables or disables the ReAct pattern
func (a *CommandForgeAgent) WithReActEnabled(enabled bool) *CommandForgeAgent {
	a.ReActEnabled = enabled
	return a
}

// AddTool adds a tool to the agent
func (a *CommandForgeAgent) AddTool(tool tools.Tool) error {
	return a.ToolCollection.AddTool(tool)
}

// AddDefaultTools adds the default tools to the agent
func (a *CommandForgeAgent) AddDefaultTools() error {
	// Add bash execution tool
	cmdTool := tools.NewBashTool("/")
	if err := a.AddTool(cmdTool); err != nil {
		return err
	}

	// Add file system tools
	fileTool := tools.NewFileTool("/")
	if err := a.AddTool(fileTool); err != nil {
		return err
	}

	// Add web search tool
	webSearchTool := tools.NewWebSearchTool("YOUR_API_KEY")
	if err := a.AddTool(webSearchTool); err != nil {
		return err
	}

	// Add web browser tool
	webBrowserTool := tools.NewWebBrowserTool()
	if err := a.AddTool(webBrowserTool); err != nil {
		return err
	}

	return nil
}

// Initialize sets up the agent
func (a *CommandForgeAgent) Initialize(ctx context.Context) error {
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
func (a *CommandForgeAgent) Run(ctx context.Context, request *Request) (*Response, error) {
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

// processRequest processes a user request
func (a *CommandForgeAgent) processRequest(ctx context.Context, request *Request) (string, error) {
	// Initialize iteration counter (for logging purposes only)
	iterationCount := 0

	// Start the processing loop
	for {
		// Increment the iteration counter
		iterationCount++
		// Log the current iteration for debugging
		fmt.Printf("CommandForge: Processing iteration %d\n", iterationCount)

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

		// Check if the ReAct pattern is enabled and if the message contains a final answer
		if a.ReActEnabled && strings.Contains(message.Content, "Final Answer:") {
			// Extract the final answer
			finalAnswer := extractFinalAnswer(message.Content)
			return finalAnswer, nil
		}

		// Check for tool calls
		toolCalls, err := llm.ParseToolCalls(message)
		if err != nil || len(toolCalls) == 0 {
			// If ReAct is enabled, try to parse the ReAct pattern
			if a.ReActEnabled {
				action, actionInput, err := parseReActPattern(message.Content)
				if err == nil {
					// Execute the action
					var actionResult interface{}
					var actionError error

					// Parse the action input as JSON
					var actionParams map[string]interface{}
					if err := json.Unmarshal([]byte(actionInput), &actionParams); err != nil {
						// If action input isn't valid JSON, use it as a string parameter
						actionParams = map[string]interface{}{
							"input": actionInput,
						}
					}

					// Execute the tool
					actionResult, actionError = a.ToolHandler.ExecuteToolByName(ctx, action, actionParams)

					// Create an observation message
					observationContent := ""
					if actionError != nil {
						observationContent = fmt.Sprintf("Error: %v", actionError)
					} else {
						// Convert the result to a string
						resultJSON, err := json.MarshalIndent(actionResult, "", "  ")
						if err != nil {
							observationContent = fmt.Sprintf("%v", actionResult)
						} else {
							observationContent = string(resultJSON)
						}
					}

					// Add the observation to the conversation history
					a.ConversationHistory = append(a.ConversationHistory, llm.Message{
						Role:    "user",
						Content: fmt.Sprintf("Observation: %s", observationContent),
					})

					continue
				}
			}

			// If we've processed several iterations with tool calls but no final answer,
			// and the last message is a regular content message (not a tool call),
			// treat it as a final answer to avoid hitting the iteration limit
			if iterationCount >= 5 && message.Content != "" {
				fmt.Printf("CommandForge: Treating message as final answer after %d iterations\n", iterationCount)
				return message.Content, nil
			}

			// No tool calls and no ReAct pattern, just return the message content
			return message.Content, nil
		}

		// Process tool calls with special handling for streaming commands
		// Log the tool calls for debugging
		fmt.Printf("CommandForge: Processing %d tool calls\n", len(toolCalls))
		for i, tc := range toolCalls {
			fmt.Printf("CommandForge: Tool call %d: ID=%s, Name=%s\n", i, tc.ID, tc.Function.Name)
		}

		toolResults, err := a.processToolCalls(ctx, toolCalls)
		if err != nil {
			return "", fmt.Errorf("failed to process tool calls: %w", err)
		}
		
		// Check if any of the tool results are for background commands
		// If so, we'll consider this iteration complete and continue
		for i, tr := range toolResults {
			if tr.Role == "tool" && i < len(toolCalls) {
				// Only consider bash commands with background flag as true background commands
				if toolCalls[i].Function.Name == "bash" {
					// Try to parse the content as JSON to check for background flag
					var result map[string]interface{}
					if err := json.Unmarshal([]byte(tr.Content), &result); err == nil {
						// Check if this is a background command
						if bg, ok := result["background"].(bool); ok && bg {
							fmt.Printf("CommandForge: Detected background command started, marking iteration as complete\n")
							
							// Add the tool results to conversation history
							a.ConversationHistory = append(a.ConversationHistory, toolResults...)
							
							// Add a final message to indicate the background command is running
							cmdID, _ := result["command_id"].(string)
							a.ConversationHistory = append(a.ConversationHistory, llm.Message{
								Role:    "assistant",
								Content: fmt.Sprintf("Background command with ID %s is now running. You can check its status later using the command_status tool.", cmdID),
							})
							
							// Return a message to the user about the background command
							return fmt.Sprintf("Background command with ID %s has been started. You can check its status using the command_status tool.", cmdID), nil
						}
					}
				}
			}
		}

		// Log the tool results for debugging
		fmt.Printf("CommandForge: Got %d tool results\n", len(toolResults))
		for i, tr := range toolResults {
			fmt.Printf("CommandForge: Tool result %d: Role=%s, ToolCallID=%s\n", i, tr.Role, tr.ToolCallID)
			// Ensure each tool result has a valid ToolCallID that matches one of the tool calls
			if tr.Role == "tool" && tr.ToolCallID == "" {
				// If ToolCallID is missing, try to find a matching tool call
				if i < len(toolCalls) {
					tr.ToolCallID = toolCalls[i].ID
					fmt.Printf("CommandForge: Fixed missing ToolCallID: %s\n", tr.ToolCallID)
				}
			}
		}

		// Add tool results to conversation history
		a.ConversationHistory = append(a.ConversationHistory, toolResults...)

		// Check if we need to trim the conversation history
		a.trimConversationHistory()
		
		// Add a safety check to prevent excessive iterations
		if iterationCount > 100 {
			// This is just a safety measure to prevent truly infinite loops
			// but it's set high enough that it should never be reached in normal operation
			return "", fmt.Errorf("safety limit reached: exceeded 100 iterations without completing the task")
		}
	}
}

// processToolCalls processes tool calls with special handling for streaming commands
func (a *CommandForgeAgent) processToolCalls(ctx context.Context, toolCalls []llm.ToolCall) ([]llm.Message, error) {
	if len(toolCalls) == 0 {
		return nil, nil
	}

	// Process each tool call and collect results
	resultMessages := make([]llm.Message, 0, len(toolCalls))

	for _, tc := range toolCalls {
		// Check if this is a command execution tool that supports streaming
		if a.StreamingEnabled && a.isStreamingCommandTool(tc.Function.Name) {
			// Handle streaming command execution
			resultMsg, err := a.handleStreamingCommand(ctx, tc)
			if err != nil {
				// Create an error message
				resultMsg = llm.Message{
					Role:       "tool",
					Content:    fmt.Sprintf(
						"Tool execution failed: %s\nError: %v",
						tc.Function.Name,
						err,
					),
					ToolCallID: tc.ID, // Add the tool call ID to associate with the preceding tool call
				}
			}
			resultMessages = append(resultMessages, resultMsg)
		} else {
			// Standard tool execution
			result, err := a.ToolHandler.ExecuteToolByName(ctx, tc.Function.Name, tc.Args)

			// Create a result message
			message := llm.Message{
				Role:       "tool",
				Content:    "",
				ToolCallID: tc.ID, // Add the tool call ID to associate with the preceding tool call
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
func (a *CommandForgeAgent) handleStreamingCommand(ctx context.Context, tc llm.ToolCall) (llm.Message, error) {
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

	// Handle command execution errors according to the two-tier approach
	// 1. System-level execution errors (return error)
	if err != nil {
		return llm.Message{}, fmt.Errorf("failed to execute command: %w", err)
	}

	// 2. Command-level failures (non-zero exit code)
	// Check if the result has an exit code field
	resultMap, ok := result.(map[string]interface{})
	if ok {
		exitCode, hasExitCode := resultMap["exitCode"].(int)
		if hasExitCode && exitCode != 0 {
			// Command ran but failed with non-zero exit code
			// Include both output and error in the result
			resultMap["success"] = false
			resultMap["error"] = fmt.Sprintf("Command failed with exit code %d", exitCode)
		} else {
			resultMap["success"] = true
		}
		result = resultMap
	}

	// Format the result as a message
	resultJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return llm.Message{
			Role:       "tool",
			Content:    fmt.Sprintf("%v", result),
			ToolCallID: tc.ID, // Add the tool call ID to associate with the preceding tool call
		}, nil
	}

	return llm.Message{
		Role:       "tool",
		Content:    string(resultJSON),
		ToolCallID: tc.ID, // Add the tool call ID to associate with the preceding tool call
	}, nil
}

// isStreamingCommandTool checks if a tool supports streaming
func (a *CommandForgeAgent) isStreamingCommandTool(toolName string) bool {
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
func (a *CommandForgeAgent) trimConversationHistory() {
	// If the history is smaller than the maximum size, do nothing
	if len(a.ConversationHistory) <= a.MaxHistorySize {
		return
	}

	// Keep the system message and the most recent messages
	newHistory := make([]llm.Message, 0, a.MaxHistorySize)

	// Always keep the system message
	newHistory = append(newHistory, a.ConversationHistory[0])

	// Create a map to track tool calls and their responses
	toolCallMap := make(map[string]bool)
	
	// First pass: identify all tool call IDs that have tool responses
	for _, msg := range a.ConversationHistory {
		if msg.Role == "tool" && msg.ToolCallID != "" {
			toolCallMap[msg.ToolCallID] = true
		}
	}

	// Calculate how many messages we can keep
	remainingSlots := a.MaxHistorySize - 1 // -1 for the system message
	// Keep the most recent messages, ensuring tool calls and responses stay together
	start := len(a.ConversationHistory) - remainingSlots
	if start < 1 {
		start = 1 // Ensure we don't include the system message again
	}

	// Create a list of messages to add, preserving tool call/response pairs
	messagesToAdd := make([]llm.Message, 0, remainingSlots)
	for i := start; i < len(a.ConversationHistory); i++ {
		msg := a.ConversationHistory[i]
		
		// Always include non-tool messages
		if msg.Role != "tool" {
			messagesToAdd = append(messagesToAdd, msg)
			continue
		}
		
		// For tool messages, only include if we have the corresponding tool call
		if msg.Role == "tool" && msg.ToolCallID != "" {
			// Check if we have the corresponding tool call in our history
			hasToolCall := false
			for j := 1; j < len(a.ConversationHistory); j++ {
				assistantMsg := a.ConversationHistory[j]
				if assistantMsg.Role == "assistant" && assistantMsg.ToolCalls != nil {
					for _, tc := range assistantMsg.ToolCalls {
						if tc.ID == msg.ToolCallID {
							hasToolCall = true
							break
						}
					}
				}
				if hasToolCall {
					break
				}
			}
			
			// Only include tool messages that have a corresponding tool call
			if hasToolCall {
				messagesToAdd = append(messagesToAdd, msg)
			}
		}
	}

	// Add the filtered messages to our history
	newHistory = append(newHistory, messagesToAdd...)

	a.ConversationHistory = newHistory
}

// ExecuteTool executes a tool by name
func (a *CommandForgeAgent) ExecuteTool(ctx context.Context, name string, params map[string]interface{}) (interface{}, error) {
	return a.ToolHandler.ExecuteToolByName(ctx, name, params)
}

// GetConversationHistory returns the conversation history
func (a *CommandForgeAgent) GetConversationHistory() []llm.Message {
	return a.ConversationHistory
}

// SaveConversation saves the conversation history to memory
func (a *CommandForgeAgent) SaveConversation(ctx context.Context, key string) error {
	// Convert the conversation history to JSON
	conversationJSON, err := json.Marshal(a.ConversationHistory)
	if err != nil {
		return fmt.Errorf("failed to marshal conversation history: %w", err)
	}

	// Save the conversation history to memory
	return a.Memory.Save(ctx, key, string(conversationJSON))
}

// LoadConversation loads conversation history from memory
func (a *CommandForgeAgent) LoadConversation(ctx context.Context, key string) error {
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
