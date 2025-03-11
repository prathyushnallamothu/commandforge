package agent

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// BaseAgent provides common functionality for all agents
type BaseAgent struct {
	Name        string
	Description string
	State       State
	Memory      Memory
	Tools       []Tool
	StateMutex  sync.RWMutex
	StartTime   time.Time
}

// Memory interface for agent memory management
type Memory interface {
	// Save stores a memory item
	Save(ctx context.Context, key string, value interface{}) error
	
	// Load retrieves a memory item
	Load(ctx context.Context, key string) (interface{}, error)
	
	// List returns all memory keys
	List(ctx context.Context) ([]string, error)
	
	// Delete removes a memory item
	Delete(ctx context.Context, key string) error
}

// Tool interface for agent tools
type Tool interface {
	// GetName returns the name of the tool
	GetName() string
	
	// GetDescription returns a description of the tool
	GetDescription() string
	
	// Execute runs the tool with the given parameters
	Execute(ctx context.Context, params map[string]interface{}) (interface{}, error)
}

// NewBaseAgent creates a new base agent
func NewBaseAgent(name, description string, memory Memory) *BaseAgent {
	return &BaseAgent{
		Name:        name,
		Description: description,
		State:       StateIdle,
		Memory:      memory,
		Tools:       make([]Tool, 0),
	}
}

// Initialize sets up the base agent
func (a *BaseAgent) Initialize(ctx context.Context) error {
	a.StateMutex.Lock()
	defer a.StateMutex.Unlock()
	
	a.State = StateIdle
	a.StartTime = time.Now()
	
	return nil
}

// GetState returns the current state of the agent
func (a *BaseAgent) GetState() State {
	a.StateMutex.RLock()
	defer a.StateMutex.RUnlock()
	
	return a.State
}

// setState sets the agent's state (internal use only)
func (a *BaseAgent) setState(state State) {
	a.StateMutex.Lock()
	defer a.StateMutex.Unlock()
	
	a.State = state
}

// AddTool adds a tool to the agent
func (a *BaseAgent) AddTool(tool Tool) {
	a.StateMutex.Lock()
	defer a.StateMutex.Unlock()
	
	a.Tools = append(a.Tools, tool)
}

// GetTools returns all tools available to the agent
func (a *BaseAgent) GetTools() []Tool {
	a.StateMutex.RLock()
	defer a.StateMutex.RUnlock()
	
	// Return a copy to avoid race conditions
	tools := make([]Tool, len(a.Tools))
	copy(tools, a.Tools)
	
	return tools
}

// GetToolByName returns a tool by its name
func (a *BaseAgent) GetToolByName(name string) (Tool, error) {
	a.StateMutex.RLock()
	defer a.StateMutex.RUnlock()
	
	for _, tool := range a.Tools {
		if tool.GetName() == name {
			return tool, nil
		}
	}
	
	return nil, fmt.Errorf("tool not found: %s", name)
}

// SaveMemory stores a memory item
func (a *BaseAgent) SaveMemory(ctx context.Context, key string, value interface{}) error {
	if a.Memory == nil {
		return fmt.Errorf("memory not initialized")
	}
	
	return a.Memory.Save(ctx, key, value)
}

// LoadMemory retrieves a memory item
func (a *BaseAgent) LoadMemory(ctx context.Context, key string) (interface{}, error) {
	if a.Memory == nil {
		return nil, fmt.Errorf("memory not initialized")
	}
	
	return a.Memory.Load(ctx, key)
}

// ListMemory returns all memory keys
func (a *BaseAgent) ListMemory(ctx context.Context) ([]string, error) {
	if a.Memory == nil {
		return nil, fmt.Errorf("memory not initialized")
	}
	
	return a.Memory.List(ctx)
}

// DeleteMemory removes a memory item
func (a *BaseAgent) DeleteMemory(ctx context.Context, key string) error {
	if a.Memory == nil {
		return fmt.Errorf("memory not initialized")
	}
	
	return a.Memory.Delete(ctx, key)
}

// Stop gracefully stops the agent
func (a *BaseAgent) Stop(ctx context.Context) error {
	a.setState(StateStopped)
	return nil
}
