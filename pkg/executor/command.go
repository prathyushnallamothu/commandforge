package executor

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// CommandResult represents the result of a command execution
type CommandResult struct {
	Success    bool     `json:"success"`
	ExitCode   int      `json:"exit_code"`
	Output     string   `json:"output"`
	Error      string   `json:"error"`
	StartTime  string   `json:"start_time"`
	EndTime    string   `json:"end_time"`
	Duration   float64  `json:"duration"`
	OutputList []string `json:"output_list,omitempty"`
	ErrorList  []string `json:"error_list,omitempty"`
}

// Command represents a command to be executed
type Command struct {
	Cmd       string
	Args      []string
	Dir       string
	Env       []string
	Timeout   time.Duration
	Streaming bool
	
	// For streaming output
	outputMu   sync.Mutex
	outputList []string
	errorList  []string
}

// NewCommand creates a new command instance
func NewCommand(cmd string, args []string, dir string) *Command {
	return &Command{
		Cmd:        cmd,
		Args:       args,
		Dir:        dir,
		Timeout:    60 * time.Second,
		Streaming:  false,
		outputList: make([]string, 0),
		errorList:  make([]string, 0),
	}
}

// NewShellCommand creates a new shell command instance
func NewShellCommand(cmdStr string, dir string) *Command {
	return &Command{
		Cmd:        "sh",
		Args:       []string{"-c", cmdStr},
		Dir:        dir,
		Timeout:    60 * time.Second,
		Streaming:  false,
		outputList: make([]string, 0),
		errorList:  make([]string, 0),
	}
}

// WithTimeout sets the timeout for the command
func (c *Command) WithTimeout(timeout time.Duration) *Command {
	c.Timeout = timeout
	return c
}

// WithEnv sets the environment variables for the command
func (c *Command) WithEnv(env []string) *Command {
	c.Env = env
	return c
}

// WithStreaming enables streaming output for the command
func (c *Command) WithStreaming() *Command {
	c.Streaming = true
	return c
}

// AppendOutput adds a line to the output list
func (c *Command) AppendOutput(line string) {
	c.outputMu.Lock()
	defer c.outputMu.Unlock()
	c.outputList = append(c.outputList, line)
}

// AppendError adds a line to the error list
func (c *Command) AppendError(line string) {
	c.outputMu.Lock()
	defer c.outputMu.Unlock()
	c.errorList = append(c.errorList, line)
}

// GetOutputAndError returns the current output and error
func (c *Command) GetOutputAndError() ([]string, []string) {
	c.outputMu.Lock()
	defer c.outputMu.Unlock()
	
	// Return copies to avoid race conditions
	outputCopy := make([]string, len(c.outputList))
	errorCopy := make([]string, len(c.errorList))
	
	copy(outputCopy, c.outputList)
	copy(errorCopy, c.errorList)
	
	return outputCopy, errorCopy
}

// Execute runs the command and returns the result
func (c *Command) Execute() (*CommandResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
	defer cancel()
	
	startTime := time.Now()
	startTimeStr := startTime.Format(time.RFC3339)
	
	// Create the command
	cmd := exec.CommandContext(ctx, c.Cmd, c.Args...)
	if c.Dir != "" {
		cmd.Dir = c.Dir
	}
	if len(c.Env) > 0 {
		cmd.Env = c.Env
	}
	
	// Set up pipes for output and error
	var stdoutBuf, stderrBuf bytes.Buffer
	var stdout, stderr io.ReadCloser
	var err error
	
	if c.Streaming {
		stdout, err = cmd.StdoutPipe()
		if err != nil {
			return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
		}
		
		stderr, err = cmd.StderrPipe()
		if err != nil {
			return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
		}
	} else {
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf
	}
	
	// Start the command
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start command: %w", err)
	}
	
	// If streaming, process the output in real-time
	if c.Streaming {
		var wg sync.WaitGroup
		wg.Add(2)
		
		// Process stdout
		go func() {
			defer wg.Done()
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				line := scanner.Text()
				c.AppendOutput(line)
				stdoutBuf.WriteString(line + "\n")
			}
		}()
		
		// Process stderr
		go func() {
			defer wg.Done()
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				line := scanner.Text()
				c.AppendError(line)
				stderrBuf.WriteString(line + "\n")
			}
		}()
		
		// Wait for output processing to complete
		wg.Wait()
	}
	
	// Wait for the command to finish
	err = cmd.Wait()
	endTime := time.Now()
	endTimeStr := endTime.Format(time.RFC3339)
	duration := endTime.Sub(startTime).Seconds()
	
	// Prepare the result
	result := &CommandResult{
		Success:    err == nil,
		ExitCode:   0,
		Output:     strings.TrimSpace(stdoutBuf.String()),
		Error:      strings.TrimSpace(stderrBuf.String()),
		StartTime:  startTimeStr,
		EndTime:    endTimeStr,
		Duration:   duration,
	}
	
	// Get exit code if available
	if exitError, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitError.ExitCode()
	}
	
	// Add output and error lists if streaming was enabled
	if c.Streaming {
		outputList, errorList := c.GetOutputAndError()
		result.OutputList = outputList
		result.ErrorList = errorList
	}
	
	// Check if the context was canceled due to timeout
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("command timed out after %v seconds", c.Timeout.Seconds())
	}
	
	// Return the result with a nil error, even if the command failed
	// This implements the two-tier error handling approach:
	// - System-level errors (like timeouts) return an error
	// - Command-level failures return a result with success=false
	return result, nil
}
