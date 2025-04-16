package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DeepSeekClient implements the LLM client interface for DeepSeek
type DeepSeekClient struct {
	APIKey     string
	Model      string
	BaseURL    string
	Timeout    time.Duration
	HTTPClient *http.Client
}

// NewDeepSeekClient creates a new DeepSeek client
func NewDeepSeekClient(apiKey, model string) *DeepSeekClient {
	return &DeepSeekClient{
		APIKey:     apiKey,
		Model:      model,
		BaseURL:    "https://api.deepseek.com/v1",
		Timeout:    60 * time.Second,
		HTTPClient: &http.Client{},
	}
}

// WithTimeout sets the timeout for API requests
func (c *DeepSeekClient) WithTimeout(timeout time.Duration) *DeepSeekClient {
	c.Timeout = timeout
	return c
}

// WithBaseURL sets a custom base URL for API requests
func (c *DeepSeekClient) WithBaseURL(baseURL string) *DeepSeekClient {
	c.BaseURL = baseURL
	return c
}

// GetModelName returns the name of the model being used
func (c *DeepSeekClient) GetModelName() string {
	return c.Model
}

// GetProvider returns the name of the LLM provider
func (c *DeepSeekClient) GetProvider() string {
	return "deepseek"
}

// ChatCompletion generates a chat completion using the DeepSeek API
func (c *DeepSeekClient) ChatCompletion(ctx context.Context, request *ChatCompletionRequest) (*ChatCompletionResponse, error) {
	// Override the model with the client's model
	request.Model = c.Model

	// Create a context with timeout
	ctxWithTimeout, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	// Marshal the request to JSON
	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create the HTTP request
	url := fmt.Sprintf("%s/chat/completions", c.BaseURL)
	req, err := http.NewRequestWithContext(ctxWithTimeout, "POST", url, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.APIKey))

	// Send the request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check for error status code
	if resp.StatusCode != http.StatusOK {
		var errorResp struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
			} `json:"error"`
		}

		if err := json.Unmarshal(body, &errorResp); err == nil && errorResp.Error.Message != "" {
			return nil, fmt.Errorf("DeepSeek API error (%s): %s", errorResp.Error.Type, errorResp.Error.Message)
		}

		return nil, fmt.Errorf("DeepSeek API returned status code %d: %s", resp.StatusCode, string(body))
	}

	// Parse the response
	var response ChatCompletionResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &response, nil
}
