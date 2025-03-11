package flow

import (
	"fmt"

	"github.com/prathyushnallamothu/commandforge/pkg/agent"
	"github.com/prathyushnallamothu/commandforge/pkg/llm"
)

// FlowType represents the type of flow
type FlowType string

// Flow types
const (
	FlowTypePlanning FlowType = "planning"
	FlowTypeSimple   FlowType = "simple"
)

// FlowFactory creates flows
type FlowFactory struct {
	LLMClient    llm.Client
	Memory       Memory
	AgentFactory *agent.Factory
}

// NewFlowFactory creates a new flow factory
func NewFlowFactory(llmClient llm.Client, memory Memory, agentFactory *agent.Factory) *FlowFactory {
	return &FlowFactory{
		LLMClient:    llmClient,
		Memory:       memory,
		AgentFactory: agentFactory,
	}
}

// CreateFlow creates a flow of the specified type
func (f *FlowFactory) CreateFlow(flowType FlowType) (Flow, error) {
	switch flowType {
	case FlowTypePlanning:
		return NewPlanningFlow(f.LLMClient, f.Memory, f.AgentFactory), nil
	case FlowTypeSimple:
		return NewSimpleFlow(f.LLMClient, f.Memory), nil
	default:
		return nil, fmt.Errorf("unknown flow type: %s", flowType)
	}
}
