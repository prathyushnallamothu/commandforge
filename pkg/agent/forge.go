package agent

import (
	"context"
	"encoding/json"
	"fmt"

	uri "net/url"

	"github.com/prathyushnallamothu/commandforge/pkg/llm"
	"github.com/prathyushnallamothu/commandforge/pkg/tools"
)

// ForgeAgent is the main agent implementation
type ForgeAgent struct {
	*BaseAgent
	LLMClient           llm.Client
	ToolHandler         *llm.ToolCallingHandler
	ToolCollection      *tools.ToolCollection
	ConversationHistory []llm.Message
	MaxHistorySize      int
	SystemPrompt        string
}

// NewForgeAgent creates a new forge agent
func NewForgeAgent(name string, llmClient llm.Client, memory Memory) *ForgeAgent {
	// Create tool collection
	toolCollection := tools.NewToolCollection()

	// Create tool handler
	toolHandler := llm.NewToolCallingHandler(toolCollection)

	// Create base agent
	baseAgent := NewBaseAgent(
		name,
		"A versatile agent for executing commands and performing tasks",
		memory,
	)

	// Create forge agent
	agent := &ForgeAgent{
		BaseAgent:           baseAgent,
		LLMClient:           llmClient,
		ToolHandler:         toolHandler,
		ToolCollection:      toolCollection,
		ConversationHistory: make([]llm.Message, 0),
		MaxHistorySize:      50,
		SystemPrompt:        defaultSystemPrompt,
	}

	return agent
}

// defaultSystemPrompt is the default system prompt for the agent
const defaultSystemPrompt = `You are CommandForge, a highly autonomous AI agent designed to help users execute commands and perform tasks.

You have access to various tools that allow you to interact with the system, including:
- Running bash commands
- Executing Python code
- Managing files
- Searching the web using the Tavily API
- Browsing web pages

When asked to perform a task, you MUST:
1. Understand the user's request thoroughly
2. Plan the necessary steps to accomplish the task
3. Execute the steps using the available tools proactively
4. ALWAYS provide a detailed response with your findings and conclusions
5. Format your response in a clear, structured way

You are designed to be FULLY AUTONOMOUS. This means:
- Take initiative to solve problems without requiring guidance
- Use multiple tools in sequence to accomplish complex tasks
- When searching for information, always use the web_search tool
- After searching, synthesize the information into a comprehensive response
- Always return useful information to the user, never an empty response

When processing search queries:
1. Use the web_search tool to find relevant information
2. Analyze and summarize the search results
3. Provide a detailed response that answers the user's query
4. Include relevant facts, examples, and context

You should be careful and responsible, especially when executing commands that might modify the system.

When you're unsure about something, use your tools to gather more information before proceeding.`

// WithSystemPrompt sets a custom system prompt
func (a *ForgeAgent) WithSystemPrompt(prompt string) *ForgeAgent {
	a.SystemPrompt = prompt
	return a
}

// WithMaxHistorySize sets the maximum conversation history size
func (a *ForgeAgent) WithMaxHistorySize(size int) *ForgeAgent {
	a.MaxHistorySize = size
	return a
}

// AddTool adds a tool to the agent
func (a *ForgeAgent) AddTool(tool tools.Tool) error {
	return a.ToolCollection.AddTool(tool)
}

// Initialize sets up the agent
func (a *ForgeAgent) Initialize(ctx context.Context) error {
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
func (a *ForgeAgent) Run(ctx context.Context, request *Request) (*Response, error) {
	// Update agent state
	a.setState(StateRunning)

	// Add user message to conversation history
	a.ConversationHistory = append(a.ConversationHistory, llm.Message{
		Role:    "user",
		Content: request.Input,
	})

	// Prepare the response
	response := &Response{
		Success:  true,
		Metadata: make(map[string]interface{}),
	}

	// Process the request
	output, err := a.processRequest(ctx, request)
	if err != nil {
		a.setState(StateError)
		response.Success = false
		response.Error = err.Error()
		return response, nil
	}

	// Update the response
	response.Output = output

	// Update agent state
	a.setState(StateIdle)

	return response, nil
}

// processRequest processes a user request
func (a *ForgeAgent) processRequest(ctx context.Context, request *Request) (string, error) {
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

	// Track tool execution failures for better fallback handling
	type toolExecutionResult struct {
		toolCall llm.ToolCall
		success  bool
		error    error
		results  []llm.Message
	}

	// Process tool calls in a loop to handle sequential tool calling
	maxIterations := 5 // Prevent infinite loops
	for i := 0; i < maxIterations; i++ {
		// Process each tool call individually and collect all results
		var allToolResults []llm.Message
		var executionResults []toolExecutionResult
		var hasFailures bool

		for _, tc := range toolCalls {
			// Process a single tool call
			toolResults, err := a.ToolHandler.ProcessToolCalls(ctx, []llm.ToolCall{tc})

			// Record the execution result regardless of success/failure
			execResult := toolExecutionResult{
				toolCall: tc,
				success:  err == nil,
				error:    err,
				results:  toolResults,
			}
			executionResults = append(executionResults, execResult)

			if err != nil {
				// Handle tool execution failure
				hasFailures = true

				// Create an error message for the failed tool call
				errorMessage := llm.Message{
					Role:       "tool",
					Content:    fmt.Sprintf("Error executing %s tool: %v", tc.Function.Name, err),
					ToolCallID: tc.ID,
				}

				// Special handling for web_browser tool failures
				if tc.Function.Name == "web_browser" {
					// Try to fallback to web_search if browser navigation fails
					var args map[string]interface{}
					json.Unmarshal([]byte(tc.Function.Arguments), &args)

					if url, ok := args["url"].(string); ok {
						// Extract domain for search context
						parsedURL, parseErr := uri.Parse(url)
						var domain string
						if parseErr == nil {
							domain = parsedURL.Hostname()
						}

						// Create a search query based on the URL
						searchParams := map[string]interface{}{
							"query": fmt.Sprintf("information about %s", url),
						}
						if domain != "" {
							searchParams["domain"] = domain
						}

						// Execute web search as fallback
						searchResults, searchErr := a.ToolHandler.ExecuteToolByName(ctx, "web_search", searchParams)
						if searchErr == nil {
							// Add fallback message
							errorMessage.Content += fmt.Sprintf("\n\nFalling back to web search for information about %s", url)

							// Add search results
							searchResultMsg := llm.Message{
								Role:       "tool",
								Content:    fmt.Sprintf("Web search results: %v", searchResults),
								ToolCallID: tc.ID,
							}
							allToolResults = append(allToolResults, searchResultMsg)
						}
					}
				}

				// Add the error message to results
				allToolResults = append(allToolResults, errorMessage)
			} else {
				// Add successful tool results
				allToolResults = append(allToolResults, toolResults...)
			}
		}

		// Add all tool results to conversation history at once
		a.ConversationHistory = append(a.ConversationHistory, allToolResults...)

		// If there were failures, add a special message to guide the LLM
		if hasFailures {
			guidanceMsg := llm.Message{
				Role:    "user",
				Content: "Some tools failed to execute. Please use the successful results to provide the best possible response, and consider alternative approaches for the failed tools.",
			}
			a.ConversationHistory = append(a.ConversationHistory, guidanceMsg)
		}

		// Create a follow-up request to process the tool results
		followUpRequest := &llm.ChatCompletionRequest{
			Messages:    a.ConversationHistory,
			Tools:       a.ToolHandler.GenerateToolDefinitions(),
			Temperature: 0.7,
		}

		// Send the follow-up request to the LLM
		followUpCompletion, err := a.LLMClient.ChatCompletion(ctx, followUpRequest)
		if err != nil {
			return "", fmt.Errorf("failed to get follow-up completion: %w", err)
		}

		// Check if there are any choices
		if len(followUpCompletion.Choices) == 0 {
			return "", fmt.Errorf("no follow-up completion choices returned")
		}

		// Get the follow-up message
		followUpMessage := followUpCompletion.Choices[0].Message

		// Add the follow-up message to the conversation history
		a.ConversationHistory = append(a.ConversationHistory, followUpMessage)

		// Trim conversation history if needed
		a.trimConversationHistory()

		// Check for more tool calls in the follow-up message
		moreCalls, _ := llm.ParseToolCalls(followUpMessage)
		if len(moreCalls) == 0 {
			// No more tool calls, return the final message content if available
			if followUpMessage.Content != "" {
				return followUpMessage.Content, nil
			}
			break // Break the loop to generate a summary if content is empty
		}

		// Continue with the next iteration of tool calls
		toolCalls = moreCalls
	}

	// If we've reached the maximum number of iterations or exited the loop without a response,
	// generate a final summary based on the conversation history
	finalRequest := &llm.ChatCompletionRequest{
		Messages: append(a.ConversationHistory, llm.Message{
			Role:    "user",
			Content: "Please provide a comprehensive summary of the information you've gathered. Make sure to include all relevant details and answer the original query thoroughly.",
		}),
		Temperature: 0.7,
	}

	// Send the final request to the LLM
	finalCompletion, err := a.LLMClient.ChatCompletion(ctx, finalRequest)
	if err != nil {
		return "I encountered an error while summarizing the information. Please try again with a more specific query.", nil
	}

	// Check if there are any choices
	if len(finalCompletion.Choices) == 0 {
		return "I couldn't generate a summary of the information. Please try again with a more specific query.", nil
	}

	// Get the final message and add it to the conversation history
	finalMessage := finalCompletion.Choices[0].Message
	a.ConversationHistory = append(a.ConversationHistory, finalMessage)

	// Return the final message content
	return finalMessage.Content, nil
}

// trimConversationHistory trims the conversation history to the maximum size
func (a *ForgeAgent) trimConversationHistory() {
	// Check if we need to trim
	if len(a.ConversationHistory) <= a.MaxHistorySize {
		return
	}

	// Keep the system message and the most recent messages
	newHistory := make([]llm.Message, 0, a.MaxHistorySize)

	// Always keep the system message (assumed to be the first message)
	newHistory = append(newHistory, a.ConversationHistory[0])

	// Keep the most recent messages
	start := len(a.ConversationHistory) - a.MaxHistorySize + 1
	newHistory = append(newHistory, a.ConversationHistory[start:]...)

	// Update the conversation history
	a.ConversationHistory = newHistory
}

// ExecuteTool executes a tool by name
func (a *ForgeAgent) ExecuteTool(ctx context.Context, name string, params map[string]interface{}) (interface{}, error) {
	return a.ToolHandler.ExecuteToolByName(ctx, name, params)
}

// GetConversationHistory returns the conversation history
func (a *ForgeAgent) GetConversationHistory() []llm.Message {
	return a.ConversationHistory
}

// SaveConversation saves the conversation history to memory
func (a *ForgeAgent) SaveConversation(ctx context.Context, key string) error {
	// Convert conversation history to JSON
	data, err := json.Marshal(a.ConversationHistory)
	if err != nil {
		return fmt.Errorf("failed to marshal conversation history: %w", err)
	}

	// Save to memory
	return a.SaveMemory(ctx, key, string(data))
}

// LoadConversation loads conversation history from memory
func (a *ForgeAgent) LoadConversation(ctx context.Context, key string) error {
	// Load from memory
	data, err := a.LoadMemory(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to load conversation history: %w", err)
	}

	// Convert to string
	dataStr, ok := data.(string)
	if !ok {
		return fmt.Errorf("invalid conversation history format")
	}

	// Parse JSON
	var history []llm.Message
	if err := json.Unmarshal([]byte(dataStr), &history); err != nil {
		return fmt.Errorf("failed to unmarshal conversation history: %w", err)
	}

	// Update conversation history
	a.ConversationHistory = history

	return nil
}
