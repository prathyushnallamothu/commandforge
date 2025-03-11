package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// WebSearchTool provides functionality to search the web
type WebSearchTool struct {
	*BaseTool
	APIKey     string
	SearchURL  string
	MaxResults int
	Timeout    time.Duration
}

// SearchResult represents a single search result
type SearchResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

// SearchResponse represents the response from a web search
type SearchResponse struct {
	Success bool           `json:"success"`
	Query   string         `json:"query"`
	Results []SearchResult `json:"results"`
	Error   string         `json:"error,omitempty"`
}

// NewWebSearchTool creates a new web search tool using Tavily API
func NewWebSearchTool(apiKey string) *WebSearchTool {
	return &WebSearchTool{
		BaseTool: NewBaseTool(
			"web_search",
			"Search the web for information using Tavily API",
		),
		APIKey:     apiKey,
		SearchURL:  "https://api.tavily.com/search",
		MaxResults: 5,
		Timeout:    10 * time.Second,
	}
}

// WithMaxResults sets the maximum number of results to return
func (t *WebSearchTool) WithMaxResults(max int) *WebSearchTool {
	t.MaxResults = max
	return t
}

// WithTimeout sets the timeout for search requests
func (t *WebSearchTool) WithTimeout(timeout time.Duration) *WebSearchTool {
	t.Timeout = timeout
	return t
}

// Execute performs a web search using Tavily API
func (t *WebSearchTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// Log the incoming parameters for debugging
	fmt.Printf("WebSearchTool Execute called with params: %+v\n", params)
	
	// Get the query from parameters
	query, ok := params["query"].(string)
	if !ok || query == "" {
		return nil, fmt.Errorf("query parameter is required and must be a string")
	}
	
	// Log the query being searched
	fmt.Printf("WebSearchTool searching for: %s\n", query)
	
	// Check for optional domain parameter
	var includeDomains []string
	if domain, ok := params["domain"].(string); ok && domain != "" {
		includeDomains = []string{domain}
	}
	
	// Create a context with timeout
	ctxWithTimeout, cancel := context.WithTimeout(ctx, t.Timeout)
	defer cancel()
	
	// Prepare the request
	reqBody, err := json.Marshal(map[string]interface{}{
		"query":          query,
		"search_depth":    "basic",
		"include_domains": includeDomains,
		"max_results":     t.MaxResults,
		"include_answer":  true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}
	
	req, err := http.NewRequestWithContext(
		ctxWithTimeout,
		"POST",
		t.SearchURL,
		strings.NewReader(string(reqBody)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+t.APIKey)
	
	// Log the request details (without the API key for security)
	fmt.Printf("WebSearchTool sending request to: %s\n", t.SearchURL)
	fmt.Printf("WebSearchTool request body: %s\n", string(reqBody))
	
	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	
	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		errorMsg := fmt.Sprintf("Tavily API returned status: %d, body: %s", resp.StatusCode, string(body))
		fmt.Printf("WebSearchTool error: %s\n", errorMsg)
		return &SearchResponse{
			Success: false,
			Query:   query,
			Error:   errorMsg,
		}, nil
	}
	
	// Log successful response status
	fmt.Printf("WebSearchTool received successful response with status: %d\n", resp.StatusCode)
	
	// Parse the response
	var tavilyResponse struct {
		Results []struct {
			Title   string  `json:"title"`
			URL     string  `json:"url"`
			Content string  `json:"content"`
			Score   float64 `json:"score"`
		} `json:"results"`
		Answer string `json:"answer"`
		Query  string `json:"query"`
	}
	
	// Read the response body for logging
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	
	// Log the response body (truncated if too large)
	respBodyStr := string(respBody)
	if len(respBodyStr) > 1000 {
		fmt.Printf("WebSearchTool received response (truncated): %s...\n", respBodyStr[:1000])
	} else {
		fmt.Printf("WebSearchTool received response: %s\n", respBodyStr)
	}
	
	// Parse the JSON response
	if err := json.Unmarshal(respBody, &tavilyResponse); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	
	// Convert Tavily results to our standard format
	results := make([]SearchResult, 0, len(tavilyResponse.Results))
	for _, item := range tavilyResponse.Results {
		results = append(results, SearchResult{
			Title:       item.Title,
			URL:         item.URL,
			Description: item.Content,
		})
		
		if len(results) >= t.MaxResults {
			break
		}
	}
	
	// If there's an answer, add it as the first result
	if tavilyResponse.Answer != "" {
		answerResult := SearchResult{
			Title:       "AI-Generated Answer",
			URL:         "",  // No URL for the answer
			Description: tavilyResponse.Answer,
		}
		// Insert at the beginning
		results = append([]SearchResult{answerResult}, results...)
		
		// Log the answer for debugging
		fmt.Printf("Tavily API returned answer: %s\n", tavilyResponse.Answer)
	}
	
	// Log the number of results and answer status
	fmt.Printf("WebSearchTool found %d results for query: %s\n", len(results), query)
	if tavilyResponse.Answer != "" {
		fmt.Printf("WebSearchTool has AI answer: %t\n", true)
	} else {
		fmt.Printf("WebSearchTool has AI answer: %t\n", false)
	}
	
	// Create the final response
	response := &SearchResponse{
		Success: true,
		Query:   query,
		Results: results,
	}
	
	// Log the final response structure
	fmt.Printf("WebSearchTool returning response with %d results\n", len(response.Results))
	
	return response, nil
}

// FallbackWebSearchTool provides a simpler web search implementation
// that doesn't require an API key but has limited functionality
type FallbackWebSearchTool struct {
	*BaseTool
	MaxResults int
	Timeout    time.Duration
}

// NewFallbackWebSearchTool creates a new fallback web search tool
func NewFallbackWebSearchTool() *FallbackWebSearchTool {
	return &FallbackWebSearchTool{
		BaseTool: NewBaseTool(
			"web_search_fallback",
			"Search the web for information (limited functionality)",
		),
		MaxResults: 5,
		Timeout:    10 * time.Second,
	}
}

// Execute performs a web search using a simple approach
func (t *FallbackWebSearchTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// Get the query from parameters
	query, ok := params["query"].(string)
	if !ok || query == "" {
		return nil, fmt.Errorf("query parameter is required and must be a string")
	}
	
	// Create a context with timeout - we don't use it directly but it's good practice
	// to have a timeout for external requests
	_, cancel := context.WithTimeout(ctx, t.Timeout)
	defer cancel()
	
	// Prepare the search URL
	searchURL := fmt.Sprintf("https://www.google.com/search?q=%s", url.QueryEscape(query))
	
	// Create a response with just the search URL
	return &SearchResponse{
		Success: true,
		Query:   query,
		Results: []SearchResult{
			{
				Title:       "Web Search Results",
				URL:         searchURL,
				Description: "Please visit this URL to see the search results",
			},
		},
	}, nil
}
