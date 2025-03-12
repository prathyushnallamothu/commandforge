package tools

import (
	"context"
	"fmt"

	"github.com/prathyushnallamothu/commandforge/pkg/executor"
)

// CommandStatusTool provides functionality to check the status of background commands
type CommandStatusTool struct {
	*BaseTool
}

// CommandStatusResult represents the result of a command status check
type CommandStatusResult struct {
	CommandID  string   `json:"command_id"`
	Running    bool     `json:"running"`
	Success    bool     `json:"success"`
	ExitCode   int      `json:"exit_code,omitempty"`
	Output     string   `json:"output"`
	Error      string   `json:"error"`
	OutputList []string `json:"output_list,omitempty"`
	ErrorList  []string `json:"error_list,omitempty"`
}

// NewCommandStatusTool creates a new command status tool
func NewCommandStatusTool() *CommandStatusTool {
	return &CommandStatusTool{
		BaseTool: NewBaseTool(
			"command_status",
			"Check the status of a background command",
		),
	}
}

// Execute checks the status of a background command
func (t *CommandStatusTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// Get the command ID from parameters
	cmdID, ok := params["command_id"].(string)
	if !ok || cmdID == "" {
		return nil, fmt.Errorf("command_id parameter is required and must be a string")
	}

	// Get the command from the registry
	bgCmd, err := executor.GetCommand(cmdID)
	if err != nil {
		return nil, err
	}

	// Get the status
	done, exitCode, output, errorStr, outputLines, errorLines := bgCmd.GetStatus()

	// Create a structured result with proper fields
	result := &CommandStatusResult{
		CommandID:  cmdID,
		Running:    !done,
		ExitCode:   exitCode,
		Output:     output,
		Error:      errorStr,
		OutputList: outputLines,
		ErrorList:  errorLines,
	}

	// Set success field based on the two-tier approach from StartIt application
	// Command-level failures (non-zero exit code) are considered a different category
	// from system-level execution errors
	if done {
		// Command has completed, set success based on exit code
		result.Success = (exitCode == 0)
	} else {
		// Command is still running, consider it successful so far
		result.Success = true
	}

	return result, nil
}
