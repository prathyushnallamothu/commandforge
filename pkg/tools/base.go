package tools

import (
	"context"
	"fmt"
)

// Tool interface defines the contract for all tools
type Tool interface {
	// GetName returns the name of the tool
	GetName() string
	
	// GetDescription returns a description of the tool
	GetDescription() string
	
	// Execute runs the tool with the given parameters
	Execute(ctx context.Context, params map[string]interface{}) (interface{}, error)
}

// BaseTool provides common functionality for all tools
type BaseTool struct {
	Name        string
	Description string
}

// GetName returns the name of the tool
func (t *BaseTool) GetName() string {
	return t.Name
}

// GetDescription returns a description of the tool
func (t *BaseTool) GetDescription() string {
	return t.Description
}

// Execute is a placeholder that should be overridden by specific tools
func (t *BaseTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	return nil, fmt.Errorf("execute not implemented for base tool")
}

// NewBaseTool creates a new base tool
func NewBaseTool(name, description string) *BaseTool {
	return &BaseTool{
		Name:        name,
		Description: description,
	}
}
