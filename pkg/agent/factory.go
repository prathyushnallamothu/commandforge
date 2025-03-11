package agent

import (
	"github.com/prathyushnallamothu/commandforge/pkg/llm"
)

// AgentType represents the type of agent to create
type AgentType string

const (
	// AgentTypeForge is the main CommandForge agent
	AgentTypeForge AgentType = "forge"
	// AgentTypeReAct is a reasoning agent using the ReAct pattern
	AgentTypeReAct AgentType = "react"
	// AgentTypeToolCall is an agent specialized for tool execution
	AgentTypeToolCall AgentType = "toolcall"
)

// Factory creates and configures agents
type Factory struct {
	LLMClient llm.Client
	Memory    Memory
}

// NewFactory creates a new agent factory
func NewFactory(llmClient llm.Client, memory Memory) *Factory {
	return &Factory{
		LLMClient: llmClient,
		Memory:    memory,
	}
}

// CreateAgent creates an agent of the specified type
func (f *Factory) CreateAgent(agentType AgentType) Agent {
	switch agentType {
	case AgentTypeForge:
		return NewForgeAgent("forge", f.LLMClient, f.Memory)
	case AgentTypeReAct:
		// Create a ReAct agent for reasoning
		return NewReActAgent("react", f.LLMClient, f.Memory)
	case AgentTypeToolCall:
		// Create a ToolCall agent for executing tools
		return NewToolCallAgent("toolcall", f.LLMClient, f.Memory)
	default:
		// Default to forge agent
		return NewForgeAgent("forge", f.LLMClient, f.Memory)
	}
}
