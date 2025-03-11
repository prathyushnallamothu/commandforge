package executor

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// BackgroundCommand represents a command running in the background with streaming output
type BackgroundCommand struct {
	ID          string
	Command     string
	WorkingDir  string
	Cmd         *exec.Cmd
	StartTime   time.Time
	EndTime     time.Time
	Duration    time.Duration
	ExitCode    int
	Output      string
	Error       string
	OutputLines []string
	ErrorLines  []string
	Done        chan struct{}
	Cancel      func()
	mu          sync.RWMutex
}

// NewBackgroundCommand creates a new background command
func NewBackgroundCommand(command string, workingDir string) (*BackgroundCommand, error) {
	// Create a unique ID for the command
	id := fmt.Sprintf("cmd-%d", time.Now().UnixNano())

	// Create the background command
	return &BackgroundCommand{
		ID:          id,
		Command:     command,
		WorkingDir:  workingDir,
		OutputLines: make([]string, 0),
		ErrorLines:  make([]string, 0),
		Done:        make(chan struct{}),
	}, nil
}

// Start begins execution of the background command
func (c *BackgroundCommand) Start() error {
	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	c.Cancel = cancel

	// Create the command
	c.Cmd = exec.CommandContext(ctx, "sh", "-c", c.Command)
	if c.WorkingDir != "" {
		c.Cmd.Dir = c.WorkingDir
	}

	// Set up pipes for stdout and stderr
	stdoutPipe, err := c.Cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderrPipe, err := c.Cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the command
	c.StartTime = time.Now()
	if err := c.Cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	// Start goroutines to read stdout and stderr
	go c.readOutput(stdoutPipe, true)
	go c.readOutput(stderrPipe, false)

	// Start goroutine to wait for command completion
	go func() {
		// Wait for the command to finish
		err := c.Cmd.Wait()
		c.EndTime = time.Now()
		c.Duration = c.EndTime.Sub(c.StartTime)

		// Get the exit code
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				c.ExitCode = exitErr.ExitCode()
			} else {
				c.ExitCode = 1
			}
		} else {
			c.ExitCode = 0
		}

		// Finalize the output and error strings
		c.mu.Lock()
		c.Output = strings.Join(c.OutputLines, "\n")
		c.Error = strings.Join(c.ErrorLines, "\n")
		c.mu.Unlock()

		// Signal that the command is done
		close(c.Done)
	}()

	return nil
}

// ExecuteCommandWithStreaming runs a command in the background with streaming output
func ExecuteCommandWithStreaming(command string, workingDir string) (*BackgroundCommand, error) {
	// Create a unique ID for the command
	id := fmt.Sprintf("cmd-%d", time.Now().UnixNano())

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	// Create the command
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	if workingDir != "" {
		cmd.Dir = workingDir
	}

	// Create pipes for stdout and stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Create the background command
	bgCmd := &BackgroundCommand{
		ID:          id,
		Command:     command,
		WorkingDir:  workingDir,
		Cmd:         cmd,
		StartTime:   time.Now(),
		OutputLines: make([]string, 0),
		ErrorLines:  make([]string, 0),
		Done:        make(chan struct{}),
		Cancel:      cancel,
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to start command: %w", err)
	}
	
	// Register the command in the registry
	RegisterCommand(bgCmd)

	// Start goroutines to read from stdout and stderr
	var wg sync.WaitGroup
	wg.Add(2)

	// Process stdout
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			bgCmd.AppendOutput(line)
		}
	}()

	// Process stderr
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			bgCmd.AppendError(line)
		}
	}()

	// Wait for the command to complete
	go func() {
		// Wait for output processing to complete
		wg.Wait()

		// Wait for the command to finish
		err := cmd.Wait()
		bgCmd.EndTime = time.Now()
		bgCmd.Duration = bgCmd.EndTime.Sub(bgCmd.StartTime)

		// Get the exit code
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				bgCmd.ExitCode = exitErr.ExitCode()
			} else {
				bgCmd.ExitCode = 1
			}
		} else {
			bgCmd.ExitCode = 0
		}

		// Finalize the output and error strings
		bgCmd.mu.Lock()
		bgCmd.Output = strings.Join(bgCmd.OutputLines, "\n")
		bgCmd.Error = strings.Join(bgCmd.ErrorLines, "\n")
		bgCmd.mu.Unlock()

		// Signal that the command is done
		close(bgCmd.Done)
	}()

	return bgCmd, nil
}

// ExecuteShellCommandWithStreaming runs a shell command in the background with streaming output
func ExecuteShellCommandWithStreaming(cmdStr string, workingDir string) (*BackgroundCommand, error) {
	return ExecuteCommandWithStreaming(cmdStr, workingDir)
}

// readOutput reads from a pipe and appends to the appropriate output list
func (c *BackgroundCommand) readOutput(pipe io.Reader, isStdout bool) {
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		line := scanner.Text()
		if isStdout {
			c.AppendOutput(line)
		} else {
			c.AppendError(line)
		}
	}
}

// AppendOutput adds a line to the output list
func (c *BackgroundCommand) AppendOutput(line string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.OutputLines = append(c.OutputLines, line)
}

// AppendError adds a line to the error list
func (c *BackgroundCommand) AppendError(line string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ErrorLines = append(c.ErrorLines, line)
}

// GetStatus returns the current status of the command
func (c *BackgroundCommand) GetStatus() (bool, int, string, string, []string, []string) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Make copies of the output and error lines to avoid race conditions
	outputCopy := make([]string, len(c.OutputLines))
	errorCopy := make([]string, len(c.ErrorLines))
	copy(outputCopy, c.OutputLines)
	copy(errorCopy, c.ErrorLines)

	// Check if the command is done
	select {
	case <-c.Done:
		// Command is done
		return true, c.ExitCode, c.Output, c.Error, outputCopy, errorCopy
	default:
		// Command is still running
		return false, 0, strings.Join(outputCopy, "\n"), strings.Join(errorCopy, "\n"), outputCopy, errorCopy
	}
}
