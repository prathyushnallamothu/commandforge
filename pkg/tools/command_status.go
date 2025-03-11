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

	// Return the status
	return &CommandStatusResult{
		CommandID:  cmdID,
		Running:    !done,
		ExitCode:   exitCode,
		Output:     output,
		Error:      errorStr,
		OutputList: outputLines,
		ErrorList:  errorLines,
	}, nil
}
