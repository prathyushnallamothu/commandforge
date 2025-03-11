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

// Session represents an interactive command session
type Session struct {
	Cmd        *exec.Cmd
	Stdin      io.WriteCloser
	Stdout     io.ReadCloser
	Stderr     io.ReadCloser
	Ctx        context.Context
	Cancel     context.CancelFunc
	OutputChan chan string
	ErrorChan  chan string
	DoneChan   chan struct{}
	Mutex      sync.Mutex
	IsRunning  bool
	StartTime  time.Time
}

// NewSession creates a new interactive session
func NewSession(command string, args []string, dir string) (*Session, error) {
	ctx, cancel := context.WithCancel(context.Background())
	
	cmd := exec.CommandContext(ctx, command, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}
	
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
	
	session := &Session{
		Cmd:        cmd,
		Stdin:      stdin,
		Stdout:     stdout,
		Stderr:     stderr,
		Ctx:        ctx,
		Cancel:     cancel,
		OutputChan: make(chan string, 100),
		ErrorChan:  make(chan string, 100),
		DoneChan:   make(chan struct{}),
		IsRunning:  false,
	}
	
	return session, nil
}

// NewShellSession creates a new interactive shell session
func NewShellSession(dir string) (*Session, error) {
	return NewSession("sh", nil, dir)
}

// Start begins the interactive session
func (s *Session) Start() error {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()
	
	if s.IsRunning {
		return fmt.Errorf("session is already running")
	}
	
	// Start the command
	if err := s.Cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}
	
	s.IsRunning = true
	s.StartTime = time.Now()
	
	// Start goroutines to read from stdout and stderr
	go s.readOutput()
	go s.readError()
	
	// Start goroutine to wait for command completion
	go func() {
		s.Cmd.Wait()
		s.Mutex.Lock()
		s.IsRunning = false
		s.Mutex.Unlock()
		close(s.DoneChan)
	}()
	
	return nil
}

// readOutput reads from stdout and sends to the output channel
func (s *Session) readOutput() {
	scanner := bufio.NewScanner(s.Stdout)
	for scanner.Scan() {
		select {
		case <-s.Ctx.Done():
			return
		case s.OutputChan <- scanner.Text():
			// Line sent to channel
		}
	}
}

// readError reads from stderr and sends to the error channel
func (s *Session) readError() {
	scanner := bufio.NewScanner(s.Stderr)
	for scanner.Scan() {
		select {
		case <-s.Ctx.Done():
			return
		case s.ErrorChan <- scanner.Text():
			// Line sent to channel
		}
	}
}

// SendCommand sends a command to the session
func (s *Session) SendCommand(cmd string) error {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()
	
	if !s.IsRunning {
		return fmt.Errorf("session is not running")
	}
	
	// Ensure the command ends with a newline
	if !strings.HasSuffix(cmd, "\n") {
		cmd += "\n"
	}
	
	_, err := io.WriteString(s.Stdin, cmd)
	if err != nil {
		return fmt.Errorf("failed to send command: %w", err)
	}
	
	return nil
}

// GetOutput retrieves output up to the current point
func (s *Session) GetOutput(timeout time.Duration) ([]string, []string, error) {
	output := []string{}
	errors := []string{}
	
	timeoutChan := time.After(timeout)
	
	for {
		select {
		case <-s.Ctx.Done():
			return output, errors, nil
		case <-s.DoneChan:
			return output, errors, nil
		case <-timeoutChan:
			return output, errors, nil
		case line := <-s.OutputChan:
			output = append(output, line)
		case line := <-s.ErrorChan:
			errors = append(errors, line)
		default:
			// No more immediate output
			if len(output) > 0 || len(errors) > 0 {
				return output, errors, nil
			}
			// Wait a bit for more output
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// Stop terminates the session
func (s *Session) Stop() error {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()
	
	if !s.IsRunning {
		return nil
	}
	
	s.Cancel()
	s.IsRunning = false
	
	return nil
}

// IsActive returns whether the session is still running
func (s *Session) IsActive() bool {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()
	return s.IsRunning
}
