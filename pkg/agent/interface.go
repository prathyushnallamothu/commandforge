package agent

import (
	"context"
)

// State represents the current state of an agent
type State string

const (
	// StateIdle indicates the agent is not currently processing anything
	StateIdle State = "idle"
	// StateRunning indicates the agent is actively processing a request
	StateRunning State = "running"
	// StateError indicates the agent encountered an error
	StateError State = "error"
	// StateStopped indicates the agent has been stopped
	StateStopped State = "stopped"
)

// Request represents a request to an agent
type Request struct {
	// The input text for the agent to process
	Input string `json:"input"`
	// Optional context for the request
	Context map[string]interface{} `json:"context,omitempty"`
}

// Response represents a response from an agent
type Response struct {
	// The output text from the agent
	Output string `json:"output"`
	// Whether the request was successful
	Success bool `json:"success"`
	// Any error message
	Error string `json:"error,omitempty"`
	// Additional metadata about the response
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// Agent defines the interface for all agents in the system
type Agent interface {
	// Initialize sets up the agent with any necessary resources
	Initialize(ctx context.Context) error
	
	// Run processes a request and returns a response
	Run(ctx context.Context, request *Request) (*Response, error)
	
	// GetState returns the current state of the agent
	GetState() State
	
	// Stop gracefully stops the agent and releases resources
	Stop(ctx context.Context) error
}
