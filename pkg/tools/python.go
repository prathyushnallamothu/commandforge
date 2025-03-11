package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/prathyushnallamothu/commandforge/pkg/executor"
)

// PythonTool provides functionality to execute Python code
type PythonTool struct {
	*BaseTool
	WorkingDir string
	Timeout    time.Duration
	PythonPath string
}

// PythonResult represents the result of Python code execution
type PythonResult struct {
	Success    bool     `json:"success"`
	ExitCode   int      `json:"exit_code"`
	Output     string   `json:"output"`
	Error      string   `json:"error"`
	Duration   float64  `json:"duration"`
	OutputList []string `json:"output_list,omitempty"`
	ErrorList  []string `json:"error_list,omitempty"`
}

// NewPythonTool creates a new Python execution tool
func NewPythonTool(workingDir string) *PythonTool {
	return &PythonTool{
		BaseTool: NewBaseTool(
			"python",
			"Execute Python code and return the output",
		),
		WorkingDir: workingDir,
		Timeout:    60 * time.Second,
		PythonPath: "python3", // Default to python3
	}
}

// WithTimeout sets the timeout for the Python tool
func (t *PythonTool) WithTimeout(timeout time.Duration) *PythonTool {
	t.Timeout = timeout
	return t
}

// WithPythonPath sets the Python executable path
func (t *PythonTool) WithPythonPath(pythonPath string) *PythonTool {
	t.PythonPath = pythonPath
	return t
}

// Execute runs Python code
func (t *PythonTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// Get the code from parameters
	code, ok := params["code"].(string)
	if !ok || code == "" {
		return nil, fmt.Errorf("code parameter is required and must be a string")
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

	// Get optional timeout
	timeout := t.Timeout
	if timeoutSec, ok := params["timeout"].(float64); ok && timeoutSec > 0 {
		timeout = time.Duration(timeoutSec) * time.Second
	}

	// Create a temporary file for the Python code
	tempFile, err := os.CreateTemp(workingDir, "python_*.py")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer os.Remove(tempFile.Name())

	// Write the code to the temporary file
	if _, err := tempFile.WriteString(code); err != nil {
		tempFile.Close()
		return nil, fmt.Errorf("failed to write code to temporary file: %w", err)
	}
	tempFile.Close()

	// Create and execute the command
	cmd := executor.NewCommand(
		t.PythonPath,
		[]string{filepath.Base(tempFile.Name())},
		workingDir,
	).WithTimeout(timeout)

	if streaming {
		cmd = cmd.WithStreaming()
	}

	result, err := cmd.Execute()
	if err != nil {
		// System-level error (not Python execution failure)
		return nil, fmt.Errorf("failed to execute Python code: %w", err)
	}

	// Convert to PythonResult
	pythonResult := &PythonResult{
		Success:    result.Success,
		ExitCode:   result.ExitCode,
		Output:     result.Output,
		Error:      result.Error,
		Duration:   result.Duration,
		OutputList: result.OutputList,
		ErrorList:  result.ErrorList,
	}

	return pythonResult, nil
}
