package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
	"github.com/prathyushnallamothu/commandforge/pkg/executor"
)

// Client represents an API client
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewClient creates a new API client
func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// CreateFlow creates a new flow
func (c *Client) CreateFlow(flowType, goal string) (string, error) {
	// Create request body
	reqBody, err := json.Marshal(map[string]string{
		"type": flowType,
		"goal": goal,
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create request
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/flows", c.BaseURL), bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("server returned error: %s", body)
	}

	// Parse response
	var response struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	return response.ID, nil
}

// ExecuteCommand executes a command in a flow
func (c *Client) ExecuteCommand(flowID, command string) (string, error) {
	// Create request body
	reqBody, err := json.Marshal(CommandRequest{
		Command: command,
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create request
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/flows/%s/execute", c.BaseURL, flowID), bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("server returned error: %s", body)
	}

	// Parse response
	var response CommandResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if !response.Success {
		return "", fmt.Errorf("command failed: %s", response.Error)
	}

	return response.CommandID, nil
}

// GetCommandStatus gets the status of a command
func (c *Client) GetCommandStatus(flowID, commandID string) (*executor.BackgroundCommandStatus, error) {
	// Create request
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/flows/%s/commands/%s", c.BaseURL, flowID, commandID), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Send request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned error: %s", body)
	}

	// Parse response
	var response CommandStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Convert to BackgroundCommandStatus
	return &executor.BackgroundCommandStatus{
		Running:    response.Running,
		ExitCode:   response.ExitCode,
		Output:     response.Output,
		Error:      response.Error,
		OutputList: response.OutputList,
		ErrorList:  response.ErrorList,
		Duration:   response.Duration,
	}, nil
}

// StreamCommandStatus streams the status of a command
func (c *Client) StreamCommandStatus(flowID, commandID string, callback func(*executor.BackgroundCommandStatus)) error {
	// Create WebSocket URL
	u, err := url.Parse(fmt.Sprintf("%s/api/v1/flows/%s/commands/%s/stream", c.BaseURL, flowID, commandID))
	if err != nil {
		return fmt.Errorf("failed to parse URL: %w", err)
	}

	// Convert http:// to ws:// and https:// to wss://
	if u.Scheme == "http" {
		u.Scheme = "ws"
	} else if u.Scheme == "https" {
		u.Scheme = "wss"
	}

	// Connect to WebSocket
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to connect to WebSocket: %w", err)
	}
	defer conn.Close()

	// Track accumulated output for incremental updates
	accumulatedOutputList := []string{}
	accumulatedErrorList := []string{}

	// Read messages
	for {
		// Read message
		var response CommandStatusResponse
		if err := conn.ReadJSON(&response); err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				break
			}
			return fmt.Errorf("failed to read message: %w", err)
		}

		// Handle incremental updates by appending to accumulated lists
		if response.Incremental {
			// Append new output and error lines to accumulated lists
			accumulatedOutputList = append(accumulatedOutputList, response.OutputList...)
			accumulatedErrorList = append(accumulatedErrorList, response.ErrorList...)

			// Use accumulated lists for the status
			response.OutputList = accumulatedOutputList
			response.ErrorList = accumulatedErrorList
		}

		// Convert to BackgroundCommandStatus
		status := &executor.BackgroundCommandStatus{
			Running:    response.Running,
			ExitCode:   response.ExitCode,
			Output:     response.Output,
			Error:      response.Error,
			OutputList: response.OutputList,
			ErrorList:  response.ErrorList,
			Duration:   response.Duration,
		}

		// Call callback
		callback(status)

		// If this is the final update, break the loop
		if response.Complete {
			break
		}

		// If command is no longer running, break
		if !response.Running {
			break
		}
	}

	return nil
}
