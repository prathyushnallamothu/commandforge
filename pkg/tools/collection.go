package tools

import (
	"context"
	"fmt"
	"sync"
)

// ToolCollection manages a collection of tools
type ToolCollection struct {
	tools map[string]Tool
	mutex sync.RWMutex
}

// NewToolCollection creates a new tool collection
func NewToolCollection() *ToolCollection {
	return &ToolCollection{
		tools: make(map[string]Tool),
	}
}

// AddTool adds a tool to the collection
func (c *ToolCollection) AddTool(tool Tool) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	name := tool.GetName()
	if _, exists := c.tools[name]; exists {
		return fmt.Errorf("tool with name '%s' already exists", name)
	}
	
	c.tools[name] = tool
	return nil
}

// GetTool retrieves a tool by name
func (c *ToolCollection) GetTool(name string) (Tool, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	
	tool, exists := c.tools[name]
	if !exists {
		return nil, fmt.Errorf("tool with name '%s' not found", name)
	}
	
	return tool, nil
}

// ListTools returns a list of all available tools
func (c *ToolCollection) ListTools() []Tool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	
	tools := make([]Tool, 0, len(c.tools))
	for _, tool := range c.tools {
		tools = append(tools, tool)
	}
	
	return tools
}

// GetToolDescriptions returns a map of tool names to descriptions
func (c *ToolCollection) GetToolDescriptions() map[string]string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	
	descriptions := make(map[string]string, len(c.tools))
	for name, tool := range c.tools {
		descriptions[name] = tool.GetDescription()
	}
	
	return descriptions
}

// ExecuteTool executes a tool by name with the given parameters
func (c *ToolCollection) ExecuteTool(ctx context.Context, name string, params map[string]interface{}) (interface{}, error) {
	tool, err := c.GetTool(name)
	if err != nil {
		return nil, err
	}
	
	return tool.Execute(ctx, params)
}

// RemoveTool removes a tool from the collection
func (c *ToolCollection) RemoveTool(name string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	if _, exists := c.tools[name]; !exists {
		return fmt.Errorf("tool with name '%s' not found", name)
	}
	
	delete(c.tools, name)
	return nil
}
