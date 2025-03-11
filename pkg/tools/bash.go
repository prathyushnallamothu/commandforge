package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/prathyushnallamothu/commandforge/pkg/executor"
)

// BashTool provides functionality to execute bash commands
type BashTool struct {
	*BaseTool
	WorkingDir string
	Timeout    time.Duration
}

// BashResult represents the result of a bash command execution
type BashResult struct {
	Success    bool     `json:"success"`
	ExitCode   int      `json:"exit_code"`
	Output     string   `json:"output"`
	Error      string   `json:"error"`
	Duration   float64  `json:"duration"`
	OutputList []string `json:"output_list,omitempty"`
	ErrorList  []string `json:"error_list,omitempty"`
}

// NewBashTool creates a new bash tool
func NewBashTool(workingDir string) *BashTool {
	return &BashTool{
		BaseTool: NewBaseTool(
			"bash",
			"Execute bash commands and return the output",
		),
		WorkingDir: workingDir,
		Timeout:    60 * time.Second,
	}
}

// WithTimeout sets the timeout for the bash tool
func (t *BashTool) WithTimeout(timeout time.Duration) *BashTool {
	t.Timeout = timeout
	return t
}

// Execute runs a bash command
func (t *BashTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// Get the command from parameters
	cmdStr, ok := params["command"].(string)
	if !ok || cmdStr == "" {
		return nil, fmt.Errorf("command parameter is required and must be a string")
	}

	// Get optional working directory
	workingDir := t.WorkingDir
	if dir, ok := params["working_dir"].(string); ok && dir != "" {
		workingDir = dir
	}

	// Get optional streaming flag
	streaming := false
	if stream, ok := params["streaming"].(bool); ok {
		streaming = stream
	}

	// Get optional background flag
	background := false
	if bg, ok := params["background"].(bool); ok {
		background = bg
	}

	// Get optional timeout
	timeout := t.Timeout
	if timeoutSec, ok := params["timeout"].(float64); ok && timeoutSec > 0 {
		timeout = time.Duration(timeoutSec) * time.Second
	}

	// If background is true, run the command in the background
	if background {
		// Start the command in the background
		bgCmd, err := executor.ExecuteCommandWithStreaming(cmdStr, workingDir)
		if err != nil {
			return nil, fmt.Errorf("failed to start background command: %w", err)
		}

		// Return immediately with command ID and initial status
		return map[string]interface{}{
			"command_id": bgCmd.ID,
			"running":   true,
			"message":   fmt.Sprintf("Command started in background with ID: %s", bgCmd.ID),
		}, nil
	}

	// Create and execute the command synchronously
	cmd := executor.NewShellCommand(cmdStr, workingDir).WithTimeout(timeout)
	if streaming {
		cmd = cmd.WithStreaming()
	}

	result, err := cmd.Execute()
	if err != nil {
		// System-level error (not command failure)
		return nil, fmt.Errorf("failed to execute command: %w", err)
	}

	// Convert to BashResult
	bashResult := &BashResult{
		Success:    result.Success,
		ExitCode:   result.ExitCode,
		Output:     result.Output,
		Error:      result.Error,
		Duration:   result.Duration,
		OutputList: result.OutputList,
		ErrorList:  result.ErrorList,
	}

	return bashResult, nil
}
