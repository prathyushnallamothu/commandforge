package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/prathyushnallamothu/commandforge/pkg/tools"
)

// ToolCallingHandler manages tool calling from LLM responses
type ToolCallingHandler struct {
	ToolCollection *tools.ToolCollection
	mutex          sync.RWMutex
	resultCache    map[string]interface{}
}

// NewToolCallingHandler creates a new tool calling handler
func NewToolCallingHandler(toolCollection *tools.ToolCollection) *ToolCallingHandler {
	return &ToolCallingHandler{
		ToolCollection: toolCollection,
		resultCache:    make(map[string]interface{}),
	}
}

// ProcessToolCalls processes tool calls from an LLM response
func (h *ToolCallingHandler) ProcessToolCalls(ctx context.Context, toolCalls []ToolCall) ([]Message, error) {
	if len(toolCalls) == 0 {
		return nil, nil
	}

	// Process each tool call and collect results
	resultMessages := make([]Message, 0, len(toolCalls))

	for _, tc := range toolCalls {
		// Execute the tool
		result, err := h.executeTool(ctx, tc)

		// Create a result message
		message := Message{
			Role:    "tool",
			Content: "",
			// Add the tool_call_id field required by OpenAI
			ToolCallID: tc.ID,
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

			// Cache the result
			h.cacheResult(tc.ID, result)
		}

		resultMessages = append(resultMessages, message)
	}

	return resultMessages, nil
}

// executeTool executes a tool based on a tool call
func (h *ToolCallingHandler) executeTool(ctx context.Context, tc ToolCall) (interface{}, error) {
	// Get the tool name
	toolName := tc.Function.Name

	// Check if the tool exists
	tool, err := h.ToolCollection.GetTool(toolName)
	if err != nil {
		// Return a more informative error message for unknown tools
		return map[string]interface{}{
			"error":           fmt.Sprintf("The tool '%s' is not available. Please use only the available tools.", toolName),
			"available_tools": h.ToolCollection.GetToolDescriptions(),
		}, nil
	}

	// Execute the tool with the provided arguments
	result, err := tool.Execute(ctx, tc.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to execute tool: %w", err)
	}

	return result, nil
}

// cacheResult caches a tool execution result
func (h *ToolCallingHandler) cacheResult(id string, result interface{}) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.resultCache[id] = result
}

// GetCachedResult retrieves a cached tool execution result
func (h *ToolCallingHandler) GetCachedResult(id string) (interface{}, bool) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	result, exists := h.resultCache[id]
	return result, exists
}

// GenerateToolDefinitions generates tool definitions for the LLM
func (h *ToolCallingHandler) GenerateToolDefinitions() []ToolDefinition {
	tools := h.ToolCollection.ListTools()
	definitions := make([]ToolDefinition, 0, len(tools))

	for _, tool := range tools {
		// Create a basic tool definition
		def := ToolDefinition{
			Type: "function",
			Function: Function{
				Name:        tool.GetName(),
				Description: tool.GetDescription(),
				Parameters: Parameters{
					Type:       "object",
					Properties: make(map[string]Property),
				},
			},
		}

		// Add default parameters based on tool type
		switch tool.GetName() {
		case "bash":
			def.Function.Parameters.Properties["command"] = Property{
				Type:        "string",
				Description: "The bash command to execute",
			}
			def.Function.Parameters.Properties["working_dir"] = Property{
				Type:        "string",
				Description: "Optional working directory for the command",
			}
			def.Function.Parameters.Properties["streaming"] = Property{
				Type:        "boolean",
				Description: "Whether to stream the output in real-time",
			}
			def.Function.Parameters.Properties["background"] = Property{
				Type:        "boolean",
				Description: "Whether to run the command in the background without waiting for completion",
			}
			def.Function.Parameters.Required = []string{"command"}

		case "python":
			def.Function.Parameters.Properties["code"] = Property{
				Type:        "string",
				Description: "The Python code to execute",
			}
			def.Function.Parameters.Properties["working_dir"] = Property{
				Type:        "string",
				Description: "Optional working directory for the code execution",
			}
			def.Function.Parameters.Required = []string{"code"}

		case "file":
			def.Function.Parameters.Properties["operation"] = Property{
				Type:        "string",
				Description: "The file operation to perform (read, write, list, delete)",
				Enum:        []string{"read", "write", "list", "delete"},
			}
			def.Function.Parameters.Properties["path"] = Property{
				Type:        "string",
				Description: "The path to the file or directory",
			}
			def.Function.Parameters.Properties["content"] = Property{
				Type:        "string",
				Description: "The content to write to the file (for write operation)",
			}
			def.Function.Parameters.Required = []string{"operation"}

		case "web_search":
			def.Function.Parameters.Properties["query"] = Property{
				Type:        "string",
				Description: "The search query",
			}
			def.Function.Parameters.Required = []string{"query"}

		case "web_browser":
			def.Function.Parameters.Properties["url"] = Property{
				Type:        "string",
				Description: "The URL to browse",
			}
			def.Function.Parameters.Required = []string{"url"}
			
		case "command_status":
			def.Function.Parameters.Properties["command_id"] = Property{
				Type:        "string",
				Description: "The ID of the background command to check",
			}
			def.Function.Parameters.Required = []string{"command_id"}
			
		case "list_commands":
			// No parameters needed for list_commands
		}

		definitions = append(definitions, def)
	}

	return definitions
}

// FormatToolCallsForPrompt formats tool calls for inclusion in a prompt
func FormatToolCallsForPrompt(toolCalls []ToolCall, results []Message) string {
	if len(toolCalls) == 0 || len(results) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Tool Calls:\n")

	for i, tc := range toolCalls {
		if i >= len(results) {
			break
		}

		// Format the tool call
		sb.WriteString(fmt.Sprintf("Tool: %s\n", tc.Function.Name))
		sb.WriteString(fmt.Sprintf("Arguments: %s\n", tc.Function.Arguments))
		sb.WriteString(fmt.Sprintf("Result: %s\n\n", results[i].Content))
	}

	return sb.String()
}

// ExecuteToolByName executes a tool by name with the given parameters
func (h *ToolCallingHandler) ExecuteToolByName(ctx context.Context, name string, params map[string]interface{}) (interface{}, error) {
	log.Printf("Executing tool: %s with params: %v", name, params)

	// Get the tool
	tool, err := h.ToolCollection.GetTool(name)
	if err != nil {
		return nil, fmt.Errorf("tool not found: %s", name)
	}

	// Execute the tool
	result, err := tool.Execute(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to execute tool: %w", err)
	}

	return result, nil
}
