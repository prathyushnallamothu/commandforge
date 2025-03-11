package flow

import (
	"context"
	"fmt"
	"sync"

	"github.com/prathyushnallamothu/commandforge/pkg/executor"
)

// FlowManager coordinates multiple flows and provides a centralized way to manage them
type FlowManager struct {
	FlowFactory    *FlowFactory
	ActiveFlows    map[string]Flow
	OutputHandlers map[string][]func(string)
	mu             sync.RWMutex
}

// NewFlowManager creates a new flow manager
func NewFlowManager(flowFactory *FlowFactory) *FlowManager {
	return &FlowManager{
		FlowFactory:    flowFactory,
		ActiveFlows:    make(map[string]Flow),
		OutputHandlers: make(map[string][]func(string)),
	}
}

// CreateFlow creates a new flow and registers it with the manager
func (m *FlowManager) CreateFlow(ctx context.Context, flowType FlowType, flowID string) (Flow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if flow with this ID already exists
	if _, exists := m.ActiveFlows[flowID]; exists {
		return nil, fmt.Errorf("flow with ID %s already exists", flowID)
	}

	// Create the flow
	flow, err := m.FlowFactory.CreateFlow(flowType)
	if err != nil {
		return nil, err
	}

	// Initialize the flow
	if err := flow.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize flow: %w", err)
	}

	// Register output handlers for planning flows
	if planningFlow, ok := flow.(*PlanningFlow); ok {
		// Add our output handler to the planning flow
		planningFlow.OutputListeners = append(planningFlow.OutputListeners, func(output string) {
			m.handleFlowOutput(flowID, output)
		})
	}

	// Store the flow
	m.ActiveFlows[flowID] = flow

	return flow, nil
}

// GetFlow retrieves a flow by ID
func (m *FlowManager) GetFlow(flowID string) (Flow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	flow, exists := m.ActiveFlows[flowID]
	if !exists {
		return nil, fmt.Errorf("flow with ID %s not found", flowID)
	}

	return flow, nil
}

// RemoveFlow removes a flow from the manager
func (m *FlowManager) RemoveFlow(flowID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.ActiveFlows[flowID]; !exists {
		return fmt.Errorf("flow with ID %s not found", flowID)
	}

	delete(m.ActiveFlows, flowID)
	delete(m.OutputHandlers, flowID)

	return nil
}

// RegisterOutputHandler registers a handler for flow output
func (m *FlowManager) RegisterOutputHandler(flowID string, handler func(string)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.OutputHandlers[flowID]; !exists {
		m.OutputHandlers[flowID] = make([]func(string), 0)
	}

	m.OutputHandlers[flowID] = append(m.OutputHandlers[flowID], handler)
}

// handleFlowOutput processes output from a flow and sends it to registered handlers
func (m *FlowManager) handleFlowOutput(flowID string, output string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	handlers, exists := m.OutputHandlers[flowID]
	if !exists || len(handlers) == 0 {
		return
	}

	for _, handler := range handlers {
		handler(output)
	}
}

// RunFlow runs a flow with the given request
func (m *FlowManager) RunFlow(ctx context.Context, flowID string, request *FlowRequest) (*FlowResponse, error) {
	// Get the flow
	flow, err := m.GetFlow(flowID)
	if err != nil {
		return nil, err
	}

	// Run the flow
	return flow.Run(ctx, request)
}

// GetCommandStatus retrieves the status of a background command
func (m *FlowManager) GetCommandStatus(flowID string, commandID string) (*executor.BackgroundCommandStatus, error) {
	// Get the flow
	flow, err := m.GetFlow(flowID)
	if err != nil {
		return nil, err
	}

	// Check if it's a planning flow with an execution pipeline
	planningFlow, ok := flow.(*PlanningFlow)
	if !ok {
		return nil, fmt.Errorf("flow with ID %s is not a planning flow", flowID)
	}

	// Get the command status
	return planningFlow.ExecutionPipeline.GetCommandStatus(commandID)
}

// ListFlows returns a list of all active flows
func (m *FlowManager) ListFlows() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	flowIDs := make([]string, 0, len(m.ActiveFlows))
	for id := range m.ActiveFlows {
		flowIDs = append(flowIDs, id)
	}

	return flowIDs
}
