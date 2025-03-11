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
		MaxHistorySize:      50,
		SystemPrompt:        defaultReActSystemPrompt,
		MaxIterations:       10, // Prevent infinite loops
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
- Running bash commands
- Executing Python code
- Managing files
- Searching the web
- Browsing web pages

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

		// Parse the ReAct pattern to extract the action and action input
		action, actionInput, err := parseReActPattern(message.Content)
		if err != nil {
			// If we can't parse the ReAct pattern, check for tool calls directly
			toolCalls, err := llm.ParseToolCalls(message)
			if err != nil || len(toolCalls) == 0 {
				// No tool calls and no ReAct pattern, just return the message content
				return message.Content, nil
			}

			// Process tool calls
			toolResults, err := a.ToolHandler.ProcessToolCalls(ctx, toolCalls)
			if err != nil {
				return "", fmt.Errorf("failed to process tool calls: %w", err)
			}

			// Add tool results to conversation history
			a.ConversationHistory = append(a.ConversationHistory, toolResults...)
			continue
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
	}

	// If we've reached the maximum number of iterations, return a timeout error
	return "", fmt.Errorf("reached maximum number of iterations (%d) without finding a final answer", a.MaxIterations)
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

	// Always keep the system message
	newHistory = append(newHistory, a.ConversationHistory[0])

	// Keep the most recent messages
	start := len(a.ConversationHistory) - a.MaxHistorySize + 1
	newHistory = append(newHistory, a.ConversationHistory[start:]...)

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
