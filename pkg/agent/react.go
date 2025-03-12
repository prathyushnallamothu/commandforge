package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/prathyushnallamothu/commandforge/pkg/llm"
	"github.com/prathyushnallamothu/commandforge/pkg/tools"
)

// ReActAgent implements the ReAct (Reasoning and Acting) pattern
// for more structured agent reasoning and tool use
type ReActAgent struct {
	*BaseAgent
	LLMClient           llm.Client
	ToolHandler         *llm.ToolCallingHandler
	ToolCollection      *tools.ToolCollection
	ConversationHistory []llm.Message
	MaxHistorySize      int
	SystemPrompt        string
	MaxIterations       int
}

// NewReActAgent creates a new ReAct agent
func NewReActAgent(name string, llmClient llm.Client, memory Memory) *ReActAgent {
	// Create tool collection
	toolCollection := tools.NewToolCollection()

	// Create tool handler
	toolHandler := llm.NewToolCallingHandler(toolCollection)

	// Create base agent
	baseAgent := NewBaseAgent(
		name,
		"A reasoning and acting agent that follows the ReAct pattern for structured problem-solving",
		memory,
	)

	// Create ReAct agent
	agent := &ReActAgent{
		BaseAgent:           baseAgent,
		LLMClient:           llmClient,
		ToolHandler:         toolHandler,
		ToolCollection:      toolCollection,
		ConversationHistory: make([]llm.Message, 0),
		MaxHistorySize:      1000, // Reduced from 50 to prevent token limit issues
		SystemPrompt:        defaultReActSystemPrompt,
		MaxIterations:       100, // Increased from 10 to allow more iterations for complex tasks
	}

	return agent
}

// defaultReActSystemPrompt is the default system prompt for the ReAct agent
const defaultReActSystemPrompt = `You are CommandForge, an autonomous AI agent designed to help users execute commands and perform tasks.

You follow the ReAct (Reasoning and Acting) pattern to solve problems:
1. THINK: Carefully analyze the problem and plan your approach
2. ACT: Execute relevant tools to gather information or perform actions
3. OBSERVE: Review the results of your actions
4. REPEAT: Continue the cycle until the task is complete

When reasoning, always structure your thinking as follows:
Thought: <your step-by-step reasoning process>
Action: <the tool you want to use>
Action Input: <the parameters for the tool>
Observation: <the result from executing the tool>

Finish with:
Thought: I now have the information needed to answer the user's question.
Final Answer: <your comprehensive response to the user>

You have access to various tools that allow you to interact with the system, including:
- Running bash commands (with support for background execution)
- Executing Python code
- Managing files
- Searching the web
- Browsing web pages
- Managing background commands

For long-running commands (like web servers or continuous monitoring):
1. Use the bash tool with background=true to run the command without blocking
2. The command will return a command_id that you can use to check status
3. Use the command_status tool with the command_id to check progress
4. Use the list_commands tool to see all running background commands

Be thorough in your reasoning, proactive in your actions, and clear in your final answers.`

// WithSystemPrompt sets a custom system prompt
func (a *ReActAgent) WithSystemPrompt(prompt string) *ReActAgent {
	a.SystemPrompt = prompt
	return a
}

// WithMaxHistorySize sets the maximum conversation history size
func (a *ReActAgent) WithMaxHistorySize(size int) *ReActAgent {
	a.MaxHistorySize = size
	return a
}

// WithMaxIterations sets the maximum number of reasoning iterations
func (a *ReActAgent) WithMaxIterations(iterations int) *ReActAgent {
	a.MaxIterations = iterations
	return a
}

// AddTool adds a tool to the agent
func (a *ReActAgent) AddTool(tool tools.Tool) error {
	return a.ToolCollection.AddTool(tool)
}

// Initialize sets up the agent
func (a *ReActAgent) Initialize(ctx context.Context) error {
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
func (a *ReActAgent) Run(ctx context.Context, request *Request) (*Response, error) {
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

// processRequest processes a user request using the ReAct pattern
func (a *ReActAgent) processRequest(ctx context.Context, request *Request) (string, error) {
	// Initialize iteration counter to prevent infinite loops
	iterationCount := 0

	// Start the ReAct loop
	for iterationCount < a.MaxIterations {
		// Increment the iteration counter
		iterationCount++

		// Trim conversation history before creating the chat request
		a.trimConversationHistory()

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

		// Check if the message contains a final answer
		if strings.Contains(message.Content, "Final Answer:") {
			// Extract the final answer
			finalAnswer := extractFinalAnswer(message.Content)
			return finalAnswer, nil
		}

		// Check for tool calls directly in the message
		toolCalls, toolCallsErr := llm.ParseToolCalls(message)
		if toolCallsErr == nil && len(toolCalls) > 0 {
			// Process tool calls
			toolResults, err := a.ToolHandler.ProcessToolCalls(ctx, toolCalls)
			if err != nil {
				return "", fmt.Errorf("failed to process tool calls: %w", err)
			}

			// Ensure each tool result has a valid ToolCallID that matches one of the tool calls
			for i := range toolResults {
				if toolResults[i].Role == "tool" && toolResults[i].ToolCallID == "" && i < len(toolCalls) {
					// If ToolCallID is missing, try to find a matching tool call
					toolResults[i].ToolCallID = toolCalls[i].ID
				}
			}

			// Add tool results to conversation history
			a.ConversationHistory = append(a.ConversationHistory, toolResults...)
			continue
		}

		// If no tool calls, try to parse the ReAct pattern
		action, actionInput, err := parseReActPattern(message.Content)
		if err != nil {
			// No tool calls and no ReAct pattern, just return the message content
			return message.Content, nil
		}

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
			// System-level execution error (tool couldn't be executed)
			observationContent = fmt.Sprintf("Error: %v", actionError)
		} else {
			// Check for command-level failures (non-zero exit code)
			resultMap, ok := actionResult.(map[string]interface{})
			if ok {
				exitCode, hasExitCode := resultMap["exitCode"].(float64)
				if hasExitCode && exitCode != 0 {
					// Command ran but failed with non-zero exit code
					// Include both output and error in the result
					resultMap["success"] = false
					resultMap["error"] = fmt.Sprintf("Command failed with exit code %d", int(exitCode))
					actionResult = resultMap
				} else {
					resultMap["success"] = true
				}
			}

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
	}

	// If we've reached the maximum number of iterations, return a timeout error
	return "", fmt.Errorf("safety limit reached: exceeded %d iterations without finding a final answer", a.MaxIterations)
}

// parseReActPattern parses the ReAct pattern from a message
func parseReActPattern(content string) (string, string, error) {
	// Check for the Action: pattern
	actionIndex := strings.Index(content, "\nAction:")
	if actionIndex == -1 {
		return "", "", fmt.Errorf("no Action found in message")
	}

	// Check for the Action Input: pattern
	actionInputIndex := strings.Index(content, "\nAction Input:")
	if actionInputIndex == -1 {
		return "", "", fmt.Errorf("no Action Input found in message")
	}

	// Extract the action
	action := content[actionIndex+9 : actionInputIndex]
	action = strings.TrimSpace(action)

	// Extract the action input
	actionInput := content[actionInputIndex+14:]

	// Check if there's an Observation after the Action Input
	observationIndex := strings.Index(actionInput, "\nObservation:")
	if observationIndex != -1 {
		actionInput = actionInput[:observationIndex]
	}

	actionInput = strings.TrimSpace(actionInput)

	return action, actionInput, nil
}

// extractFinalAnswer extracts the final answer from a message
func extractFinalAnswer(content string) string {
	// Check for the Final Answer: pattern
	finalAnswerIndex := strings.Index(content, "Final Answer:")
	if finalAnswerIndex == -1 {
		return content
	}

	// Extract the final answer
	finalAnswer := content[finalAnswerIndex+13:]
	return strings.TrimSpace(finalAnswer)
}

// trimConversationHistory trims the conversation history to the maximum size
func (a *ReActAgent) trimConversationHistory() {
	// If the history is smaller than the maximum size, do nothing
	if len(a.ConversationHistory) <= a.MaxHistorySize {
		return
	}

	// Keep the system message and the most recent messages
	newHistory := make([]llm.Message, 0, a.MaxHistorySize)

	// Always keep the system message (assuming it's the first message)
	systemMessage := a.ConversationHistory[0]
	newHistory = append(newHistory, systemMessage)

	// Keep the user's original request (if available)
	if len(a.ConversationHistory) > 1 {
		// Find the first user message (which is likely the original request)
		for i := 1; i < len(a.ConversationHistory); i++ {
			if a.ConversationHistory[i].Role == "user" {
				newHistory = append(newHistory, a.ConversationHistory[i])
				break
			}
		}
	}

	// Create a map to track tool calls and their responses
	toolCallMap := make(map[string]bool)
	
	// First pass: identify all tool call IDs that have tool responses
	for _, msg := range a.ConversationHistory {
		if msg.Role == "tool" && msg.ToolCallID != "" {
			toolCallMap[msg.ToolCallID] = true
		}
	}

	// Calculate how many messages we can keep
	remainingSlots := a.MaxHistorySize - len(newHistory)
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
func (a *ReActAgent) ExecuteTool(ctx context.Context, name string, params map[string]interface{}) (interface{}, error) {
	return a.ToolHandler.ExecuteToolByName(ctx, name, params)
}

// GetConversationHistory returns the conversation history
func (a *ReActAgent) GetConversationHistory() []llm.Message {
	return a.ConversationHistory
}

// SaveConversation saves the conversation history to memory
func (a *ReActAgent) SaveConversation(ctx context.Context, key string) error {
	// Convert the conversation history to JSON
	conversationJSON, err := json.Marshal(a.ConversationHistory)
	if err != nil {
		return fmt.Errorf("failed to marshal conversation history: %w", err)
	}

	// Save the conversation history to memory
	return a.Memory.Save(ctx, key, string(conversationJSON))
}

// LoadConversation loads conversation history from memory
func (a *ReActAgent) LoadConversation(ctx context.Context, key string) error {
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
