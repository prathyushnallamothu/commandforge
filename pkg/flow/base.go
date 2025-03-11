package flow

import (
	"context"
	"fmt"
	"sync"
)

// BaseFlow provides common functionality for all flows
type BaseFlow struct {
	Name        string
	Description string
	State       State
	StateMutex  sync.Mutex
	Memory      Memory
}

// NewBaseFlow creates a new base flow
func NewBaseFlow(name, description string, memory Memory) *BaseFlow {
	return &BaseFlow{
		Name:        name,
		Description: description,
		State:       StateIdle,
		Memory:      memory,
	}
}

// GetState returns the current state of the flow
func (f *BaseFlow) GetState() State {
	f.StateMutex.Lock()
	defer f.StateMutex.Unlock()
	return f.State
}

// setState sets the state of the flow
func (f *BaseFlow) setState(state State) {
	f.StateMutex.Lock()
	defer f.StateMutex.Unlock()
	f.State = state
}

// GetName returns the name of the flow
func (f *BaseFlow) GetName() string {
	return f.Name
}

// GetDescription returns the description of the flow
func (f *BaseFlow) GetDescription() string {
	return f.Description
}

// Initialize initializes the flow
func (f *BaseFlow) Initialize(ctx context.Context) error {
	// Set the flow state to idle
	f.setState(StateIdle)
	return nil
}

// Run processes a user request - to be implemented by specific flows
func (f *BaseFlow) Run(ctx context.Context, request *FlowRequest) (*FlowResponse, error) {
	return nil, fmt.Errorf("Run method not implemented")
}

// SaveState saves the flow state to memory
func (f *BaseFlow) SaveState(ctx context.Context, key string, value interface{}) error {
	if f.Memory == nil {
		return fmt.Errorf("memory not initialized")
	}
	return f.Memory.Save(ctx, key, value)
}

// LoadState loads the flow state from memory
func (f *BaseFlow) LoadState(ctx context.Context, key string) (interface{}, error) {
	if f.Memory == nil {
		return nil, fmt.Errorf("memory not initialized")
	}
	return f.Memory.Load(ctx, key)
}
