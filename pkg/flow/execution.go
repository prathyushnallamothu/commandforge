package flow

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/prathyushnallamothu/commandforge/pkg/agent"
	"github.com/prathyushnallamothu/commandforge/pkg/executor"
)

// ExecutionResult represents the result of a command execution
type ExecutionResult struct {
	Success    bool     `json:"success"`
	ExitCode   int      `json:"exit_code"`
	Output     string   `json:"output"`
	Error      string   `json:"error"`
	Duration   float64  `json:"duration"`
	OutputList []string `json:"output_list,omitempty"`
	ErrorList  []string `json:"error_list,omitempty"`
}

// StreamingExecutor handles command execution with real-time streaming output
type StreamingExecutor struct {
	CommandMutex   sync.Mutex
	ActiveCommands map[string]*executor.BackgroundCommand
}

// NewStreamingExecutor creates a new streaming executor
func NewStreamingExecutor() *StreamingExecutor {
	return &StreamingExecutor{
		ActiveCommands: make(map[string]*executor.BackgroundCommand),
	}
}

// ExecuteCommand runs a command and streams the output
func (e *StreamingExecutor) ExecuteCommand(ctx context.Context, command string, workingDir string) (*ExecutionResult, error) {
	// Create a unique ID for this command
	commandID := fmt.Sprintf("cmd-%d", time.Now().UnixNano())

	// Create and start the background command
	cmd, err := executor.ExecuteCommandWithStreaming(command, workingDir)
	if err != nil {
		// Handle system-level execution errors
		return &ExecutionResult{
			Success:  false,
			ExitCode: -1,
			Error:    fmt.Sprintf("Failed to execute command: %v", err),
		}, nil
	}

	// Store the command in the active commands map
	e.CommandMutex.Lock()
	e.ActiveCommands[commandID] = cmd
	e.CommandMutex.Unlock()

	// Wait for the command to complete or context to be canceled
	select {
	case <-cmd.Done:
		// Command completed
	case <-ctx.Done():
		// Context canceled, kill the command
		cmd.Cancel()
		<-cmd.Done
	}

	// Remove the command from the active commands map
	e.CommandMutex.Lock()
	delete(e.ActiveCommands, commandID)
	e.CommandMutex.Unlock()

	// Create the execution result
	result := &ExecutionResult{
		Success:    cmd.ExitCode == 0,
		ExitCode:   cmd.ExitCode,
		Output:     cmd.Output,
		Error:      cmd.Error,
		Duration:   cmd.Duration.Seconds(),
		OutputList: cmd.OutputLines,
		ErrorList:  cmd.ErrorLines,
	}

	return result, nil
}

// GetCommandStatus retrieves the current status of a command
func (e *StreamingExecutor) GetCommandStatus(commandID string) (*ExecutionResult, error) {
	// Get the command from the active commands map
	e.CommandMutex.Lock()
	cmd, ok := e.ActiveCommands[commandID]
	e.CommandMutex.Unlock()

	if !ok {
		return nil, fmt.Errorf("command not found: %s", commandID)
	}

	// Create the execution result
	result := &ExecutionResult{
		Success:    cmd.ExitCode == 0,
		ExitCode:   cmd.ExitCode,
		Output:     cmd.Output,
		Error:      cmd.Error,
		Duration:   cmd.Duration.Seconds(),
		OutputList: cmd.OutputLines,
		ErrorList:  cmd.ErrorLines,
	}

	return result, nil
}

// ExecutionPipeline coordinates the execution of commands
type ExecutionPipeline struct {
	Executor           *StreamingExecutor
	WorkingDir         string
	StatusListeners    []func(string, *ExecutionResult)
	BackgroundCommands map[string]*executor.BackgroundCommand
	mu                 sync.RWMutex
}

// NewExecutionPipeline creates a new execution pipeline
func NewExecutionPipeline(workingDir string) *ExecutionPipeline {
	return &ExecutionPipeline{
		Executor:           NewStreamingExecutor(),
		WorkingDir:         workingDir,
		StatusListeners:    make([]func(string, *ExecutionResult), 0),
		BackgroundCommands: make(map[string]*executor.BackgroundCommand),
	}
}

// AddStatusListener registers a function to be called when command status changes
func (p *ExecutionPipeline) AddStatusListener(listener func(string, *ExecutionResult)) {
	p.StatusListeners = append(p.StatusListeners, listener)
}

// ExecuteCommand runs a command and notifies listeners of status changes
func (p *ExecutionPipeline) ExecuteCommand(ctx context.Context, commandID string, command string) (*ExecutionResult, error) {
	// Execute the command
	result, err := p.Executor.ExecuteCommand(ctx, command, p.WorkingDir)
	if err != nil {
		return nil, err
	}

	// Notify listeners of the result
	for _, listener := range p.StatusListeners {
		listener(commandID, result)
	}

	return result, nil
}

// ExecuteCommandWithAgent runs a command using an agent and returns the result
func (p *ExecutionPipeline) ExecuteCommandWithAgent(ctx context.Context, agentInstance agent.Agent, command string) (*ExecutionResult, error) {
	// Create the request for the agent
	agentRequest := &agent.Request{
		Input: command,
	}

	// Run the agent
	response, err := agentInstance.Run(ctx, agentRequest)
	if err != nil {
		return &ExecutionResult{
			Success: false,
			Error:   fmt.Sprintf("Agent error: %v", err),
		}, nil
	}

	// Create the execution result
	result := &ExecutionResult{
		Success: response.Success,
		Output:  response.Output,
	}

	if !response.Success {
		result.Error = response.Error
	}

	return result, nil
}

// ExecuteCommandInBackground runs a command in the background and returns its ID
func (p *ExecutionPipeline) ExecuteCommandInBackground(command string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Create a background command
	cmd, err := executor.NewBackgroundCommand(command, p.WorkingDir)
	if err != nil {
		return "", fmt.Errorf("failed to create background command: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start background command: %w", err)
	}

	// Store the command
	p.BackgroundCommands[cmd.ID] = cmd

	return cmd.ID, nil
}

// GetCommandStatus retrieves the status of a background command
func (p *ExecutionPipeline) GetCommandStatus(commandID string) (*executor.BackgroundCommandStatus, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Find the command
	cmd, exists := p.BackgroundCommands[commandID]
	if !exists {
		return nil, fmt.Errorf("command with ID %s not found", commandID)
	}

	// Get the status
	running, exitCode, output, errOutput, outputList, errorList := cmd.GetStatus()

	// Calculate duration
	duration := 0.0
	if !cmd.StartTime.IsZero() {
		if cmd.EndTime.IsZero() {
			duration = time.Since(cmd.StartTime).Seconds()
		} else {
			duration = cmd.EndTime.Sub(cmd.StartTime).Seconds()
		}
	}

	// Create and return the status object
	return executor.NewBackgroundCommandStatus(
		cmd.ID,
		cmd.Command,
		cmd.WorkingDir,
		running,
		exitCode,
		output,
		errOutput,
		outputList,
		errorList,
		duration,
	), nil
}
