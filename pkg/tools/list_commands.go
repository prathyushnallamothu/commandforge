package tools

import (
	"context"

	"github.com/prathyushnallamothu/commandforge/pkg/executor"
)

// ListCommandsTool provides functionality to list all background commands
type ListCommandsTool struct {
	*BaseTool
}

// ListCommandsResult represents the result of listing commands
type ListCommandsResult struct {
	Commands []string `json:"commands"`
}

// NewListCommandsTool creates a new list commands tool
func NewListCommandsTool() *ListCommandsTool {
	return &ListCommandsTool{
		BaseTool: NewBaseTool(
			"list_commands",
			"List all background commands",
		),
	}
}

// Execute lists all background commands
func (t *ListCommandsTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// Get the list of commands
	commands := executor.ListCommands()

	// Return the list
	return &ListCommandsResult{
		Commands: commands,
	}, nil
}
