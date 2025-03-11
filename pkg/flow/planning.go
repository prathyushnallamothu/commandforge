package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/prathyushnallamothu/commandforge/pkg/agent"
	"github.com/prathyushnallamothu/commandforge/pkg/executor"
	"github.com/prathyushnallamothu/commandforge/pkg/llm"
)

// PlanStep represents a step in a plan
type PlanStep struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Command     string `json:"command,omitempty"`
	Status      string `json:"status"` // pending, running, completed, failed
	Output      string `json:"output,omitempty"`
	Error       string `json:"error,omitempty"`
}

// Plan represents a sequence of steps to accomplish a task
type Plan struct {
	Goal  string     `json:"goal"`
	Steps []PlanStep `json:"steps"`
}

// PlanningFlow coordinates multiple agents to handle complex tasks
type PlanningFlow struct {
	*BaseFlow
	LLMClient         llm.Client
	AgentFactory      *agent.Factory
	PlannerAgent      agent.Agent
	ExecutorAgent     agent.Agent
	CurrentPlan       *Plan
	ExecutionPipeline *ExecutionPipeline
	OutputListeners   []func(string)
}

// NewPlanningFlow creates a new planning flow
func NewPlanningFlow(llmClient llm.Client, memory Memory, agentFactory *agent.Factory) *PlanningFlow {
	flow := &PlanningFlow{
		BaseFlow:          NewBaseFlow("planning", "A flow that plans and executes complex tasks", memory),
		LLMClient:         llmClient,
		AgentFactory:      agentFactory,
		CurrentPlan:       nil,
		ExecutionPipeline: NewExecutionPipeline("/"),
		OutputListeners:   make([]func(string), 0),
	}

	// Create the planner agent (ReAct agent for reasoning)
	flow.PlannerAgent = agentFactory.CreateAgent(agent.AgentTypeReAct)

	// Create the executor agent (ToolCall agent for execution)
	flow.ExecutorAgent = agentFactory.CreateAgent(agent.AgentTypeToolCall)

	// Add a status listener to the execution pipeline
	flow.ExecutionPipeline.AddStatusListener(func(commandID string, result *ExecutionResult) {
		// Find the step with this command ID
		if flow.CurrentPlan != nil {
			for i := range flow.CurrentPlan.Steps {
				step := &flow.CurrentPlan.Steps[i]
				if step.ID == commandID {
					// Update the step status based on the result
					if result.Success {
						step.Status = "completed"
					} else {
						step.Status = "failed"
					}
					step.Output = result.Output
					step.Error = result.Error

					// Notify output listeners
					for _, listener := range flow.OutputListeners {
						outputUpdate := fmt.Sprintf("Step %s: %s\nStatus: %s", step.ID, step.Description, step.Status)
						if result.Output != "" {
							outputUpdate += fmt.Sprintf("\nOutput: %s", result.Output)
						}
						if result.Error != "" {
							outputUpdate += fmt.Sprintf("\nError: %s", result.Error)
						}
						listener(outputUpdate)
					}

					// Save the updated plan
					ctx := context.Background()
					if err := flow.savePlan(ctx); err != nil {
						// Just log the error, don't interrupt execution
						fmt.Printf("Error saving plan: %v\n", err)
					}
					break
				}
			}
		}
	})

	return flow
}

// Initialize initializes the flow
func (f *PlanningFlow) Initialize(ctx context.Context) error {
	// Initialize the base flow
	if err := f.BaseFlow.Initialize(ctx); err != nil {
		return err
	}

	// Initialize the planner agent
	if err := f.PlannerAgent.Initialize(ctx); err != nil {
		return err
	}

	// Initialize the executor agent
	if err := f.ExecutorAgent.Initialize(ctx); err != nil {
		return err
	}

	return nil
}

// Run processes a user request by planning and executing steps
func (f *PlanningFlow) Run(ctx context.Context, request *FlowRequest) (*FlowResponse, error) {
	// Set the flow state to running
	f.setState(StateRunning)

	// Generate a plan
	plan, err := f.generatePlan(ctx, request.Input)
	if err != nil {
		f.setState(StateError)
		return &FlowResponse{
			Output:  "",
			Success: false,
			Error:   fmt.Sprintf("Failed to generate plan: %v", err),
		}, nil
	}

	// Store the current plan
	f.CurrentPlan = plan

	// Save the plan to memory
	if err := f.savePlan(ctx); err != nil {
		f.setState(StateError)
		return &FlowResponse{
			Output:  "",
			Success: false,
			Error:   fmt.Sprintf("Failed to save plan: %v", err),
		}, nil
	}

	// Notify listeners about the plan
	planDescription := fmt.Sprintf("Generated plan for: %s\n", f.CurrentPlan.Goal)
	for i, step := range f.CurrentPlan.Steps {
		planDescription += fmt.Sprintf("Step %d: %s\n", i+1, step.Description)
		if step.Command != "" {
			planDescription += fmt.Sprintf("  Command: %s\n", step.Command)
		}
	}
	for _, listener := range f.OutputListeners {
		listener(planDescription)
	}

	// Execute the plan
	result, err := f.executePlan(ctx)
	if err != nil {
		f.setState(StateError)
		return &FlowResponse{
			Output:  "",
			Success: false,
			Error:   fmt.Sprintf("Failed to execute plan: %v", err),
		}, nil
	}

	// Set the flow state to complete
	f.setState(StateComplete)

	// Return the response
	return &FlowResponse{
		Output:  result,
		Success: true,
	}, nil
}

// generatePlan uses the planner agent to create a plan
func (f *PlanningFlow) generatePlan(ctx context.Context, input string) (*Plan, error) {
	// Create a system prompt for the planner
	systemPrompt := `You are a planning agent. Your task is to break down complex requests into a series of steps.

For each step, provide:
1. A clear description of what needs to be done
2. If applicable, a command to execute

Respond with a JSON object in the following format:
{
  "goal": "The overall goal of the plan",
  "steps": [
    {
      "id": "step1",
      "description": "Description of the step",
      "command": "Command to execute (if applicable)"
    },
    ...
  ]
}`

	// Create the request for the planner agent
	plannerRequest := &agent.Request{
		Input: input,
		Context: map[string]interface{}{
			"system_prompt": systemPrompt,
		},
	}

	// Run the planner agent
	plannerResponse, err := f.PlannerAgent.Run(ctx, plannerRequest)
	if err != nil {
		return nil, fmt.Errorf("planner agent error: %w", err)
	}

	if !plannerResponse.Success {
		return nil, fmt.Errorf("planner failed: %s", plannerResponse.Error)
	}

	// Parse the plan from the response
	plan, err := parsePlan(plannerResponse.Output)
	if err != nil {
		return nil, fmt.Errorf("failed to parse plan: %w", err)
	}

	// Initialize step statuses
	for i := range plan.Steps {
		plan.Steps[i].Status = "pending"
	}

	return plan, nil
}

// parsePlan extracts a plan from the planner's output
func parsePlan(output string) (*Plan, error) {
	// Extract JSON from the output
	jsonStart := strings.Index(output, "{")
	jsonEnd := strings.LastIndex(output, "}")

	if jsonStart == -1 || jsonEnd == -1 || jsonEnd <= jsonStart {
		return nil, fmt.Errorf("could not find valid JSON in output")
	}

	jsonStr := output[jsonStart : jsonEnd+1]

	// Parse the JSON
	var plan Plan
	if err := json.Unmarshal([]byte(jsonStr), &plan); err != nil {
		return nil, fmt.Errorf("failed to unmarshal plan: %w", err)
	}

	// Validate the plan
	if len(plan.Steps) == 0 {
		return nil, fmt.Errorf("plan contains no steps")
	}

	return &plan, nil
}

// executePlan runs each step in the plan
func (f *PlanningFlow) executePlan(ctx context.Context) (string, error) {
	if f.CurrentPlan == nil || len(f.CurrentPlan.Steps) == 0 {
		return "", fmt.Errorf("no plan to execute")
	}

	results := []string{fmt.Sprintf("Executing plan for: %s\n", f.CurrentPlan.Goal)}

	// Execute each step in sequence
	for i := range f.CurrentPlan.Steps {
		step := &f.CurrentPlan.Steps[i]
		step.Status = "running"

		// Save the updated plan
		if err := f.savePlan(ctx); err != nil {
			return "", fmt.Errorf("failed to save plan: %w", err)
		}

		results = append(results, fmt.Sprintf("\nStep %s: %s", step.ID, step.Description))

		// If the step has a command, execute it
		if step.Command != "" {
			results = append(results, fmt.Sprintf("Executing: %s", step.Command))

			// Notify output listeners that we're starting this step
			for _, listener := range f.OutputListeners {
				listener(fmt.Sprintf("Starting step %s: %s\nExecuting: %s", step.ID, step.Description, step.Command))
			}

			// Execute the command in the background with streaming output
			commandID, err := f.ExecuteCommandInBackground(step.Command)
			if err != nil {
				step.Status = "failed"
				step.Error = fmt.Sprintf("Execution error: %v", err)
				results = append(results, fmt.Sprintf("Error: %s", step.Error))

				// Save the updated plan
				if saveErr := f.savePlan(ctx); saveErr != nil {
					return strings.Join(results, "\n"), fmt.Errorf("failed to save plan: %w", saveErr)
				}

				// Continue with the next step despite the error
				continue
			}

			// Store the command ID in the step for later status checks
			step.ID = commandID

			// Mark the step as running - the actual status will be checked later
			step.Status = "running"
			results = append(results, fmt.Sprintf("Command started with ID: %s", commandID))

			// Wait for a short time to get initial output (optional)
			select {
			case <-ctx.Done():
				// Context canceled
				step.Status = "failed"
				step.Error = "Command execution was canceled"
			case <-time.After(500 * time.Millisecond):
				// Get initial status
				status, err := f.ExecutionPipeline.GetCommandStatus(commandID)
				if err == nil && len(status.OutputList) > 0 {
					outputLen := len(status.OutputList)
					linesToShow := 5
					if outputLen < linesToShow {
						linesToShow = outputLen
					}
					results = append(results, fmt.Sprintf("Initial output: %s", strings.Join(status.OutputList[:linesToShow], "\n")))
				}
			}
		} else {
			// If there's no command, mark the step as completed
			step.Status = "completed"
		}

		// Save the updated plan
		if err := f.savePlan(ctx); err != nil {
			return strings.Join(results, "\n"), fmt.Errorf("failed to save plan: %w", err)
		}
	}

	// Generate a summary of the execution
	summary := f.generateSummary(ctx)
	results = append(results, "\n"+summary)

	return strings.Join(results, "\n"), nil
}

// savePlan stores the current plan in memory
func (f *PlanningFlow) savePlan(ctx context.Context) error {
	if f.CurrentPlan == nil {
		return nil
	}

	// Convert the plan to JSON
	planJSON, err := json.Marshal(f.CurrentPlan)
	if err != nil {
		return fmt.Errorf("failed to marshal plan: %w", err)
	}

	// Save the plan to memory
	return f.SaveState(ctx, "current_plan", string(planJSON))
}

// loadPlan retrieves a stored plan from memory
func (f *PlanningFlow) loadPlan(ctx context.Context) error {
	// Load the plan from memory
	value, err := f.LoadState(ctx, "current_plan")
	if err != nil {
		return fmt.Errorf("failed to load plan: %w", err)
	}

	// Convert the value to a string
	planJSON, ok := value.(string)
	if !ok {
		return fmt.Errorf("plan is not a string")
	}

	// Parse the plan
	var plan Plan
	if err := json.Unmarshal([]byte(planJSON), &plan); err != nil {
		return fmt.Errorf("failed to unmarshal plan: %w", err)
	}

	// Set the current plan
	f.CurrentPlan = &plan

	return nil
}

// ExecuteCommandInBackground runs a command in the background and returns its ID
func (f *PlanningFlow) ExecuteCommandInBackground(command string) (string, error) {
	// Use the execution pipeline to run the command in the background
	commandID, err := f.ExecutionPipeline.ExecuteCommandInBackground(command)
	if err != nil {
		return "", fmt.Errorf("failed to execute background command: %w", err)
	}

	// Notify listeners about the command execution
	for _, listener := range f.OutputListeners {
		listener(fmt.Sprintf("Started background command: %s (ID: %s)", command, commandID))
	}

	return commandID, nil
}

// GetCommandStatusUpdates checks the status of a background command and notifies listeners
func (f *PlanningFlow) GetCommandStatusUpdates(commandID string) (*executor.BackgroundCommandStatus, error) {
	// Get the command status
	status, err := f.ExecutionPipeline.GetCommandStatus(commandID)
	if err != nil {
		return nil, fmt.Errorf("failed to get command status: %w", err)
	}

	// Notify listeners about the command status
	for _, listener := range f.OutputListeners {
		// Only notify if there's output or error to report
		if len(status.OutputList) > 0 || len(status.ErrorList) > 0 {
			var statusMsg string
			if status.Running {
				statusMsg = "Running"
			} else if status.ExitCode == 0 {
				statusMsg = "Completed successfully"
			} else {
				statusMsg = fmt.Sprintf("Failed with exit code %d", status.ExitCode)
			}

			listener(fmt.Sprintf("Command status update (ID: %s): %s\nDuration: %.2f seconds", commandID, statusMsg, status.Duration))

			// Include the latest output lines (up to 10)
			if len(status.OutputList) > 0 {
				outputLen := len(status.OutputList)
				linesToShow := 10
				if outputLen < linesToShow {
					linesToShow = outputLen
				}
				listener(fmt.Sprintf("Output: %s", strings.Join(status.OutputList[outputLen-linesToShow:], "\n")))
			}

			// Include the latest error lines (up to 5)
			if len(status.ErrorList) > 0 {
				errorLen := len(status.ErrorList)
				linesToShow := 5
				if errorLen < linesToShow {
					linesToShow = errorLen
				}
				listener(fmt.Sprintf("Error: %s", strings.Join(status.ErrorList[errorLen-linesToShow:], "\n")))
			}
		}
	}

	return status, nil
}

// generateSummary creates a summary of the plan execution
func (f *PlanningFlow) generateSummary(ctx context.Context) string {
	if f.CurrentPlan == nil {
		return "No plan available"
	}

	// Count steps by status
	total := len(f.CurrentPlan.Steps)
	completed := 0
	failed := 0

	for _, step := range f.CurrentPlan.Steps {
		if step.Status == "completed" {
			completed++
		} else if step.Status == "failed" {
			failed++
		}
	}

	// Generate the summary
	summary := fmt.Sprintf("Plan Execution Summary:\n")
	summary += fmt.Sprintf("Goal: %s\n", f.CurrentPlan.Goal)
	summary += fmt.Sprintf("Total Steps: %d\n", total)
	summary += fmt.Sprintf("Completed: %d\n", completed)
	summary += fmt.Sprintf("Failed: %d\n", failed)

	// Add overall status
	if failed > 0 {
		summary += "Status: Completed with errors"
	} else if completed == total {
		summary += "Status: Successfully completed"
	} else {
		summary += "Status: Partially completed"
	}

	// Notify output listeners of the summary
	for _, listener := range f.OutputListeners {
		listener(summary)
	}

	return summary
}
