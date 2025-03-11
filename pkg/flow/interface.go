package flow

import (
	"context"
)

// State represents the current state of a flow
type State string

// Flow states
const (
	StateIdle     State = "idle"
	StateRunning  State = "running"
	StateError    State = "error"
	StateComplete State = "complete"
)

// FlowRequest represents a request to a flow
type FlowRequest struct {
	Input string
	User  string
}

// FlowResponse represents a response from a flow
type FlowResponse struct {
	Output  string
	Success bool
	Error   string
}

// Flow defines the interface for a flow
type Flow interface {
	// Initialize initializes the flow
	Initialize(ctx context.Context) error

	// Run processes a user request
	Run(ctx context.Context, request *FlowRequest) (*FlowResponse, error)

	// GetState returns the current state of the flow
	GetState() State

	// GetName returns the name of the flow
	GetName() string

	// GetDescription returns the description of the flow
	GetDescription() string
}

// Memory interface for storing and retrieving flow state
type Memory interface {
	// Save saves a value to memory
	Save(ctx context.Context, key string, value interface{}) error

	// Load loads a value from memory
	Load(ctx context.Context, key string) (interface{}, error)
}
