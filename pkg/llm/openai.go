package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OpenAIClient implements the LLM client interface for OpenAI
type OpenAIClient struct {
	APIKey     string
	Model      string
	BaseURL    string
	Timeout    time.Duration
	HTTPClient *http.Client
}

// NewOpenAIClient creates a new OpenAI client
func NewOpenAIClient(apiKey, model string) *OpenAIClient {
	return &OpenAIClient{
		APIKey:     apiKey,
		Model:      model,
		BaseURL:    "https://api.openai.com/v1",
		Timeout:    60 * time.Second,
		HTTPClient: &http.Client{},
	}
}

// WithTimeout sets the timeout for API requests
func (c *OpenAIClient) WithTimeout(timeout time.Duration) *OpenAIClient {
	c.Timeout = timeout
	return c
}

// WithBaseURL sets a custom base URL for API requests
func (c *OpenAIClient) WithBaseURL(baseURL string) *OpenAIClient {
	c.BaseURL = baseURL
	return c
}

// GetModelName returns the name of the model being used
func (c *OpenAIClient) GetModelName() string {
	return c.Model
}

// GetProvider returns the name of the LLM provider
func (c *OpenAIClient) GetProvider() string {
	return "openai"
}

// validateConversationHistory checks if the conversation history is valid for the OpenAI API
// It ensures that all tool response messages have a corresponding tool call message
func validateConversationHistory(messages []Message) error {
	// Create a map to track tool calls
	toolCallMap := make(map[string]bool)
	
	// First pass: identify all tool call IDs
	for _, msg := range messages {
		if msg.Role == "assistant" && msg.ToolCalls != nil {
			for _, tc := range msg.ToolCalls {
				toolCallMap[tc.ID] = true
			}
		}
	}
	
	// Debug: Print all tool call IDs
	fmt.Printf("Found %d tool call IDs in conversation history\n", len(toolCallMap))
	for id := range toolCallMap {
		fmt.Printf("  Tool call ID: %s\n", id)
	}
	
	// Second pass: check if all tool responses have a corresponding tool call
	var toolResponses []string
	for i, msg := range messages {
		if msg.Role == "tool" {
			if msg.ToolCallID == "" {
				// Tool response without a tool call ID
				fmt.Printf("Warning: Tool response at index %d has no ToolCallID\n", i)
				// We'll let this pass for now, as we've added fixes to ensure this doesn't happen
			} else {
				toolResponses = append(toolResponses, msg.ToolCallID)
				if !toolCallMap[msg.ToolCallID] {
					// Print detailed error information for debugging
					fmt.Printf("Error: Tool response at index %d has invalid ToolCallID: %s\n", i, msg.ToolCallID)
					
					// Remove the invalid tool response from the conversation
					// This is a temporary fix to allow the conversation to continue
					// In a production environment, you might want to handle this differently
					// messages = append(messages[:i], messages[i+1:]...)
					// i-- // Adjust the index after removing an element
					
					// For now, just return an error
					return fmt.Errorf("tool response message at index %d has no corresponding tool call (ID: %s)", i, msg.ToolCallID)
				}
			}
		}
	}
	
	// Debug: Print all tool response IDs
	fmt.Printf("Found %d tool responses in conversation history\n", len(toolResponses))
	for _, id := range toolResponses {
		fmt.Printf("  Tool response ID: %s\n", id)
	}
	
	return nil
}

// ChatCompletion generates a chat completion using the OpenAI API
func (c *OpenAIClient) ChatCompletion(ctx context.Context, request *ChatCompletionRequest) (*ChatCompletionResponse, error) {
	// Override the model with the client's model
	request.Model = c.Model
	
	// Validate the conversation history
	if err := validateConversationHistory(request.Messages); err != nil {
		return nil, fmt.Errorf("invalid conversation history: %w", err)
	}
	
	// Create a context with timeout
	ctxWithTimeout, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	
	// Marshal the request to JSON
	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	
	// Create the HTTP request
	url := fmt.Sprintf("%s/chat/completions", c.BaseURL)
	req, err := http.NewRequestWithContext(ctxWithTimeout, "POST", url, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.APIKey))
	
	// Send the request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	
	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	
	// Check for error status code
	if resp.StatusCode != http.StatusOK {
		var errorResp struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
			} `json:"error"`
		}
		
		if err := json.Unmarshal(body, &errorResp); err == nil && errorResp.Error.Message != "" {
			return nil, fmt.Errorf("OpenAI API error (%s): %s", errorResp.Error.Type, errorResp.Error.Message)
		}
		
		return nil, fmt.Errorf("OpenAI API returned status code %d: %s", resp.StatusCode, string(body))
	}
	
	// Log the response for debugging (truncated if too large)
	responseStr := string(body)
	if len(responseStr) > 1000 {
		fmt.Printf("OpenAI response (truncated): %s...\n", responseStr[:1000])
	} else {
		fmt.Printf("OpenAI response: %s\n", responseStr)
	}
	
	// Parse the response
	var response ChatCompletionResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	
	// Log the parsed response structure
	fmt.Printf("OpenAI response parsed: model=%s, choices=%d\n", response.Model, len(response.Choices))
	if len(response.Choices) > 0 {
		message := response.Choices[0].Message
		fmt.Printf("First choice: role=%s, content_length=%d\n", message.Role, len(message.Content))
	}
	
	return &response, nil
}

// ParseToolCalls extracts tool calls from a message
func ParseToolCalls(message Message) ([]ToolCall, error) {
	// Log the message for debugging
	fmt.Printf("Parsing tool calls from message: role=%s, content_length=%d\n", message.Role, len(message.Content))
	
	// Check if the message has tool_calls directly (OpenAI format)
	if message.ToolCalls != nil && len(message.ToolCalls) > 0 {
		fmt.Printf("Found %d tool calls in message.ToolCalls\n", len(message.ToolCalls))
		
		// Convert OpenAI tool calls to our format
		toolCalls := make([]ToolCall, 0, len(message.ToolCalls))
		for _, tc := range message.ToolCalls {
			// Parse arguments as JSON
			var argsMap map[string]interface{}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &argsMap); err != nil {
				// If arguments aren't valid JSON, use empty map
				argsMap = make(map[string]interface{})
				fmt.Printf("Warning: Failed to parse tool call arguments: %v\n", err)
			}
			
			toolCalls = append(toolCalls, ToolCall{
				ID:   tc.ID,
				Type: tc.Type,
				Function: ToolCallFunction{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
				Args: argsMap,
			})
		}
		
		return toolCalls, nil
	}
	
	// Fallback: Try to parse from content as JSON
	var rawMessage map[string]interface{}
	if err := json.Unmarshal([]byte(message.Content), &rawMessage); err != nil {
		// Not JSON, so no tool calls
		fmt.Printf("Message content is not JSON, no tool calls found\n")
		return nil, nil
	}
	
	// Check for tool_calls field
	toolCallsRaw, ok := rawMessage["tool_calls"].([]interface{})
	if !ok {
		fmt.Printf("No tool_calls field found in JSON content\n")
		return nil, nil
	}
	
	fmt.Printf("Found %d tool calls in message content JSON\n", len(toolCallsRaw))
	
	// Parse tool calls
	toolCalls := make([]ToolCall, 0, len(toolCallsRaw))
	for _, tc := range toolCallsRaw {
		tcMap, ok := tc.(map[string]interface{})
		if !ok {
			continue
		}
		
		// Extract ID and type
		id, _ := tcMap["id"].(string)
		tcType, _ := tcMap["type"].(string)
		
		// Extract function
		funcMap, ok := tcMap["function"].(map[string]interface{})
		if !ok {
			continue
		}
		
		name, _ := funcMap["name"].(string)
		args, _ := funcMap["arguments"].(string)
		
		// Parse arguments as JSON
		var argsMap map[string]interface{}
		if err := json.Unmarshal([]byte(args), &argsMap); err != nil {
			// If arguments aren't valid JSON, use empty map
			argsMap = make(map[string]interface{})
		}
		
		toolCalls = append(toolCalls, ToolCall{
			ID:   id,
			Type: tcType,
			Function: ToolCallFunction{
				Name:      name,
				Arguments: args,
			},
			Args: argsMap,
		})
	}
	
	return toolCalls, nil
}
