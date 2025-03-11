package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"golang.org/x/net/html"
)

// WebBrowserTool provides functionality to browse web pages with interactive capabilities
// similar to Anthropic's computer use tool. It uses chromedp to automate browser
// interactions and can perform actions like navigating to URLs, taking screenshots,
// clicking elements, typing text, scrolling, and extracting content.
type WebBrowserTool struct {
	*BaseTool
	Timeout          time.Duration     // Timeout for browser operations
	ScreenshotDir    string            // Directory to store screenshots
	BrowserWidth     int               // Browser window width
	BrowserHeight    int               // Browser window height
	UserDataDir      string            // Directory for browser user data
	Headless         bool              // Whether to run browser in headless mode
	KeepBrowserOpen  bool              // Whether to keep the browser open between operations
	browser          *chromedp.Browser // Browser instance for reuse
	allocCtx         context.Context   // Browser allocator context
	allocCancel      context.CancelFunc // Cancel function for allocator context
	browserCtx       context.Context   // Browser context
	contentProcessor *ContentProcessor // Processor for handling content token limits
}

// BrowserAction represents the type of action to perform in the browser
type BrowserAction string

const (
	ActionNavigate     BrowserAction = "navigate"      // Navigate to a URL
	ActionScreenshot   BrowserAction = "screenshot"    // Take a screenshot
	ActionClick        BrowserAction = "click"         // Click at coordinates
	ActionType         BrowserAction = "type"          // Type text
	ActionScroll       BrowserAction = "scroll"        // Scroll the page
	ActionWait         BrowserAction = "wait"          // Wait for a specified duration
	ActionExtractText  BrowserAction = "extract_text"  // Extract text from the page
	ActionHandleDialog BrowserAction = "handle_dialog" // Handle common dialogs (cookie consent, etc.)
	ActionMultiStep    BrowserAction = "multi_step"    // Perform multiple actions in sequence
)

// BrowseResult represents the result of a web browsing operation
type BrowseResult struct {
	Success          bool   `json:"success"`
	URL              string `json:"url,omitempty"`
	Title            string `json:"title,omitempty"`
	Content          string `json:"content,omitempty"`
	Error            string `json:"error,omitempty"`
	Screenshot       string `json:"screenshot,omitempty"`        // Base64 encoded screenshot
	Coordinates      []int  `json:"coordinates,omitempty"`       // Current cursor coordinates
	Action           string `json:"action,omitempty"`            // The action that was performed
	ContentTruncated bool   `json:"content_truncated,omitempty"` // Whether content was truncated due to size
}

// BrowserParams represents the parameters for browser actions
type BrowserParams struct {
	Action      string          `json:"action,omitempty"`
	URL         string          `json:"url,omitempty"`
	Text        string          `json:"text,omitempty"`
	Coordinates []int           `json:"coordinates,omitempty"`
	Direction   string          `json:"direction,omitempty"`
	Amount      int             `json:"amount,omitempty"`
	Duration    float64         `json:"duration,omitempty"`
	Selector    string          `json:"selector,omitempty"`
	DialogType  string          `json:"dialog_type,omitempty"` // Type of dialog to handle (cookie, popup, etc.)
	Steps       []BrowserParams `json:"steps,omitempty"`       // Steps for multi-step navigation
	Retries     int             `json:"retries,omitempty"`     // Number of retries for an action
	WaitTime    float64         `json:"wait_time,omitempty"`   // Time to wait after an action in seconds
	Extra       interface{}     `json:"extra,omitempty"`
}

// MarshalJSON implements json.Marshaler for BrowseResult
func (r *BrowseResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(*r)
}

// UnmarshalJSON implements json.Unmarshaler for BrowseResult
func (r *BrowseResult) UnmarshalJSON(data []byte) error {
	type Alias BrowseResult
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(r),
	}
	return json.Unmarshal(data, aux)
}

// MarshalJSON implements json.Marshaler for BrowserParams
func (p *BrowserParams) MarshalJSON() ([]byte, error) {
	return json.Marshal(*p)
}

// UnmarshalJSON implements json.Unmarshaler for BrowserParams
func (p *BrowserParams) UnmarshalJSON(data []byte) error {
	type Alias BrowserParams
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(p),
	}
	return json.Unmarshal(data, aux)
}

// NewWebBrowserTool creates a new web browser tool with interactive capabilities
func NewWebBrowserTool() *WebBrowserTool {
	// Create temporary directory for screenshots if it doesn't exist
	screenshotDir, err := os.MkdirTemp("", "browser-screenshots-")
	if err != nil {
		screenshotDir = "./screenshots"
		os.MkdirAll(screenshotDir, 0755)
	}

	// Create temporary directory for user data if it doesn't exist
	userDataDir, err := os.MkdirTemp("", "browser-userdata-")
	if err != nil {
		userDataDir = "./browser-data"
		os.MkdirAll(userDataDir, 0755)
	}

	return &WebBrowserTool{
		BaseTool: NewBaseTool(
			"web_browser",
			"Browse web pages interactively and extract content",
		),
		Timeout:          30 * time.Second,
		ScreenshotDir:    screenshotDir,
		BrowserWidth:     1280,
		BrowserHeight:    800,
		UserDataDir:      userDataDir,
		Headless:         false,
		KeepBrowserOpen:  true,
		contentProcessor: NewContentProcessor(),
	}
}

// WithTimeout sets the timeout for web requests
func (t *WebBrowserTool) WithTimeout(timeout time.Duration) *WebBrowserTool {
	t.Timeout = timeout
	return t
}

// WithDimensions sets the browser window dimensions
func (t *WebBrowserTool) WithDimensions(width, height int) *WebBrowserTool {
	t.BrowserWidth = width
	t.BrowserHeight = height
	return t
}

// WithHeadless sets whether the browser should run in headless mode
func (t *WebBrowserTool) WithHeadless(headless bool) *WebBrowserTool {
	t.Headless = headless
	return t
}

// WithKeepBrowserOpen sets whether to keep the browser open between operations
func (t *WebBrowserTool) WithKeepBrowserOpen(keepOpen bool) *WebBrowserTool {
	t.KeepBrowserOpen = keepOpen
	return t
}

// WithContentProcessorSettings customizes the content processor settings
func (t *WebBrowserTool) WithContentProcessorSettings(maxContentLength, maxTitleLength int) *WebBrowserTool {
	if t.contentProcessor == nil {
		t.contentProcessor = NewContentProcessor()
	}

	if maxContentLength > 0 {
		t.contentProcessor.MaxContentLength = maxContentLength
	}

	if maxTitleLength > 0 {
		t.contentProcessor.MaxTitleLength = maxTitleLength
	}

	return t
}

// paramsToStruct converts a map[string]interface{} to a BrowserParams struct
func paramsToStruct(params map[string]interface{}) (*BrowserParams, error) {
	// Convert the map to JSON
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal parameters: %w", err)
	}

	// Convert JSON to BrowserParams struct
	var browserParams BrowserParams
	if err := json.Unmarshal(paramsJSON, &browserParams); err != nil {
		return nil, fmt.Errorf("failed to unmarshal parameters: %w", err)
	}

	// Set default action if not specified
	if browserParams.Action == "" {
		browserParams.Action = string(ActionNavigate)
	}

	return &browserParams, nil
}

// GetToolDefinition returns the JSON schema for the tool
func (t *WebBrowserTool) GetToolDefinition() (string, error) {
	// Define the tool schema
	schema := map[string]interface{}{
		"name":        t.Name,
		"description": t.Description,
		"parameters": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"action": map[string]interface{}{
					"type": "string",
					"enum": []string{
						string(ActionNavigate),
						string(ActionScreenshot),
						string(ActionClick),
						string(ActionType),
						string(ActionScroll),
						string(ActionWait),
						string(ActionExtractText),
						string(ActionHandleDialog),
						string(ActionMultiStep),
					},
					"description": "The browser action to perform",
				},
				"url": map[string]interface{}{
					"type":        "string",
					"description": "The URL to navigate to",
				},
				"text": map[string]interface{}{
					"type":        "string",
					"description": "The text to type",
				},
				"coordinates": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "integer",
					},
					"description": "The coordinates to click at [x, y]",
				},
				"direction": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"up", "down", "left", "right"},
					"description": "The direction to scroll",
				},
				"amount": map[string]interface{}{
					"type":        "integer",
					"description": "The amount to scroll",
				},
				"duration": map[string]interface{}{
					"type":        "number",
					"description": "The duration to wait in seconds",
				},
				"selector": map[string]interface{}{
					"type":        "string",
					"description": "The CSS selector to extract text from",
				},
				"captcha_type": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"recaptcha", "hcaptcha", "generic"},
					"description": "The type of CAPTCHA to handle (recaptcha, hcaptcha, or generic)",
				},
				"dialog_type": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"cookie", "popup", "notification", "generic"},
					"description": "The type of dialog to handle (cookie consent, popup, etc.)",
				},
				"steps": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "object",
					},
					"description": "Steps for multi-step navigation",
				},
				"retries": map[string]interface{}{
					"type":        "integer",
					"description": "Number of retries for an action",
				},
				"wait_time": map[string]interface{}{
					"type":        "number",
					"description": "Time to wait after an action in seconds",
				},
			},
		},
	}

	// Convert to JSON
	schemaJSON, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal tool definition: %w", err)
	}

	return string(schemaJSON), nil
}

// Execute performs browser actions based on the provided parameters
func (t *WebBrowserTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// Convert map to struct
	browserParams, err := paramsToStruct(params)
	if err != nil {
		return nil, err
	}

	action := BrowserAction(browserParams.Action)

	// Check if we already have a browser instance
	var browserCtx context.Context
	var opCancel context.CancelFunc
	
	if t.browserCtx == nil || !t.KeepBrowserOpen {
		// Create browser options
		opts := []chromedp.ExecAllocatorOption{
			chromedp.NoFirstRun,
			chromedp.NoDefaultBrowserCheck,
			chromedp.UserDataDir(t.UserDataDir),
			chromedp.WindowSize(t.BrowserWidth, t.BrowserHeight),
		}

		if t.Headless {
			opts = append(opts, chromedp.Headless)
		}

		// If we're keeping the browser open, store the contexts
		if t.KeepBrowserOpen {
			// Clean up any existing browser
			t.closeBrowser()
			
			// Create a new browser context
			t.allocCtx, t.allocCancel = chromedp.NewExecAllocator(ctx, opts...)
			t.browserCtx, _ = chromedp.NewContext(t.allocCtx)
			browserCtx = t.browserCtx
		} else {
			// Create a new browser context that will be closed after this operation
			allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
			defer allocCancel()
			
			browserCtx, _ = chromedp.NewContext(allocCtx)
		}
	} else {
		// Reuse existing browser context
		browserCtx = t.browserCtx
	}

	// Set a timeout just for this operation
	opCtx, opCancel := context.WithTimeout(browserCtx, t.Timeout)
	defer opCancel()

	// Initialize content processor if not already done
	if t.contentProcessor == nil {
		t.contentProcessor = NewContentProcessor()
	}

	// Handle different actions
	var result interface{}
	var actionErr error

	switch action {
	case ActionNavigate:
		result, actionErr = t.handleNavigate(opCtx, params)
	case ActionScreenshot:
		result, actionErr = t.handleScreenshot(opCtx, params)
	case ActionClick:
		result, actionErr = t.handleClick(opCtx, params)
	case ActionType:
		result, actionErr = t.handleType(opCtx, params)
	case ActionScroll:
		result, actionErr = t.handleScroll(opCtx, params)
	case ActionWait:
		result, actionErr = t.handleWait(opCtx, params)
	case ActionExtractText:
		result, actionErr = t.handleExtractText(opCtx, params)
	case ActionHandleDialog:
		result, actionErr = t.handleDialog(opCtx, params)
	case ActionMultiStep:
		result, actionErr = t.handleMultiStep(opCtx, params)
	default:
		return &BrowseResult{
			Success: false,
			Error:   fmt.Sprintf("unsupported action: %s", action),
		}, nil
	}

	// If there was an error, return it directly
	if actionErr != nil {
		return result, actionErr
	}

	// Process the result to handle token limits if it's a BrowseResult
	if browseResult, ok := result.(*BrowseResult); ok {
		// Process the result to ensure it doesn't exceed token limits
		processedResult := t.contentProcessor.ProcessBrowseResult(browseResult)
		return processedResult, nil
	}

	// Return the original result if it's not a BrowseResult
	return result, nil
}

// handleScreenshot takes a screenshot of the current page
func (t *WebBrowserTool) handleScreenshot(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// Check if URL is provided, otherwise use the current page
	urlStr, ok := params["url"].(string)
	if ok && urlStr != "" {
		// Navigate to the URL first
		if err := chromedp.Run(ctx, chromedp.Navigate(urlStr)); err != nil {
			return &BrowseResult{
				Success: false,
				URL:     urlStr,
				Error:   fmt.Sprintf("failed to navigate: %v", err),
				Action:  string(ActionScreenshot),
			}, nil
		}
	}

	// Wait for the page to load
	if err := chromedp.Run(ctx, chromedp.WaitReady("body", chromedp.ByQuery)); err != nil {
		return &BrowseResult{
			Success: false,
			Error:   fmt.Sprintf("failed to wait for page to load: %v", err),
			Action:  string(ActionScreenshot),
		}, nil
	}

	// Take a screenshot
	var screenshot []byte
	var title string
	if err := chromedp.Run(ctx,
		chromedp.Title(&title),
		chromedp.CaptureScreenshot(&screenshot),
	); err != nil {
		return &BrowseResult{
			Success: false,
			Error:   fmt.Sprintf("failed to capture screenshot: %v", err),
			Action:  string(ActionScreenshot),
		}, nil
	}

	// Encode screenshot as base64
	screenshotBase64 := base64.StdEncoding.EncodeToString(screenshot)

	// Save screenshot to file for reference
	screenshotPath := filepath.Join(t.ScreenshotDir, fmt.Sprintf("screenshot-%d.png", time.Now().UnixNano()))
	os.WriteFile(screenshotPath, screenshot, 0644)

	return &BrowseResult{
		Success:    true,
		Title:      title,
		Screenshot: screenshotBase64,
		Action:     string(ActionScreenshot),
	}, nil
}

// handleClick clicks at the specified coordinates
func (t *WebBrowserTool) handleClick(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// Get coordinates
	coordinatesRaw, ok := params["coordinates"].([]interface{})
	if !ok || len(coordinatesRaw) != 2 {
		return nil, fmt.Errorf("coordinates parameter is required and must be an array of two integers")
	}

	// Convert coordinates to integers
	x, xOk := coordinatesRaw[0].(float64)
	y, yOk := coordinatesRaw[1].(float64)
	if !xOk || !yOk {
		return nil, fmt.Errorf("coordinates must be integers")
	}

	// Click at the specified coordinates
	if err := chromedp.Run(ctx, chromedp.MouseClickXY(float64(x), float64(y))); err != nil {
		return &BrowseResult{
			Success:     false,
			Error:       fmt.Sprintf("failed to click at coordinates: %v", err),
			Coordinates: []int{int(x), int(y)},
			Action:      string(ActionClick),
		}, nil
	}

	// Take a screenshot after clicking
	var screenshot []byte
	if err := chromedp.Run(ctx, chromedp.CaptureScreenshot(&screenshot)); err != nil {
		return &BrowseResult{
			Success:     true,
			Coordinates: []int{int(x), int(y)},
			Error:       fmt.Sprintf("click successful but failed to capture screenshot: %v", err),
			Action:      string(ActionClick),
		}, nil
	}

	// Encode screenshot as base64
	screenshotBase64 := base64.StdEncoding.EncodeToString(screenshot)

	// Save screenshot to file for reference
	screenshotPath := filepath.Join(t.ScreenshotDir, fmt.Sprintf("click-%d.png", time.Now().UnixNano()))
	os.WriteFile(screenshotPath, screenshot, 0644)

	return &BrowseResult{
		Success:     true,
		Coordinates: []int{int(x), int(y)},
		Screenshot:  screenshotBase64,
		Action:      string(ActionClick),
	}, nil
}

// handleType types text at the current cursor position
func (t *WebBrowserTool) handleType(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// Get text to type
	text, ok := params["text"].(string)
	if !ok || text == "" {
		return nil, fmt.Errorf("text parameter is required and must be a string")
	}

	// Type the text
	if err := chromedp.Run(ctx, chromedp.SendKeys("body", text, chromedp.ByQuery)); err != nil {
		return &BrowseResult{
			Success: false,
			Error:   fmt.Sprintf("failed to type text: %v", err),
			Action:  string(ActionType),
		}, nil
	}

	// Take a screenshot after typing
	var screenshot []byte
	if err := chromedp.Run(ctx, chromedp.CaptureScreenshot(&screenshot)); err != nil {
		return &BrowseResult{
			Success: true,
			Content: text,
			Error:   fmt.Sprintf("typing successful but failed to capture screenshot: %v", err),
			Action:  string(ActionType),
		}, nil
	}

	// Encode screenshot as base64
	screenshotBase64 := base64.StdEncoding.EncodeToString(screenshot)

	// Save screenshot to file for reference
	screenshotPath := filepath.Join(t.ScreenshotDir, fmt.Sprintf("type-%d.png", time.Now().UnixNano()))
	os.WriteFile(screenshotPath, screenshot, 0644)

	return &BrowseResult{
		Success:    true,
		Content:    text,
		Screenshot: screenshotBase64,
		Action:     string(ActionType),
	}, nil
}

// handleScroll scrolls the page
func (t *WebBrowserTool) handleScroll(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// Get scroll direction and amount
	direction, ok := params["direction"].(string)
	if !ok || direction == "" {
		direction = "down" // Default to scrolling down
	}

	amountRaw, ok := params["amount"].(float64)
	if !ok {
		amountRaw = 300 // Default scroll amount
	}
	amount := int(amountRaw)

	// Determine scroll direction
	var scrollJS string
	switch direction {
	case "up":
		scrollJS = fmt.Sprintf("window.scrollBy(0, -%d)", amount)
	case "down":
		scrollJS = fmt.Sprintf("window.scrollBy(0, %d)", amount)
	case "left":
		scrollJS = fmt.Sprintf("window.scrollBy(-%d, 0)", amount)
	case "right":
		scrollJS = fmt.Sprintf("window.scrollBy(%d, 0)", amount)
	default:
		return nil, fmt.Errorf("invalid scroll direction: %s", direction)
	}

	// Execute scroll
	if err := chromedp.Run(ctx, chromedp.Evaluate(scrollJS, nil)); err != nil {
		return &BrowseResult{
			Success: false,
			Error:   fmt.Sprintf("failed to scroll: %v", err),
			Action:  string(ActionScroll),
		}, nil
	}

	// Wait a bit for the scroll to complete
	if err := chromedp.Run(ctx, chromedp.Sleep(100*time.Millisecond)); err != nil {
		return &BrowseResult{
			Success: false,
			Error:   fmt.Sprintf("failed to wait after scroll: %v", err),
			Action:  string(ActionScroll),
		}, nil
	}

	// Take a screenshot after scrolling
	var screenshot []byte
	if err := chromedp.Run(ctx, chromedp.CaptureScreenshot(&screenshot)); err != nil {
		return &BrowseResult{
			Success: true,
			Error:   fmt.Sprintf("scroll successful but failed to capture screenshot: %v", err),
			Action:  string(ActionScroll),
		}, nil
	}

	// Encode screenshot as base64
	screenshotBase64 := base64.StdEncoding.EncodeToString(screenshot)

	// Save screenshot to file for reference
	screenshotPath := filepath.Join(t.ScreenshotDir, fmt.Sprintf("scroll-%d.png", time.Now().UnixNano()))
	os.WriteFile(screenshotPath, screenshot, 0644)

	return &BrowseResult{
		Success:    true,
		Screenshot: screenshotBase64,
		Action:     string(ActionScroll),
	}, nil
}

// handleWait waits for a specified duration
func (t *WebBrowserTool) handleWait(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// Get wait duration
	durationRaw, ok := params["duration"].(float64)
	if !ok {
		durationRaw = 1.0 // Default to 1 second
	}

	// Convert to duration
	duration := time.Duration(durationRaw * float64(time.Second))

	// Wait for the specified duration
	if err := chromedp.Run(ctx, chromedp.Sleep(duration)); err != nil {
		return &BrowseResult{
			Success: false,
			Error:   fmt.Sprintf("failed to wait: %v", err),
			Action:  string(ActionWait),
		}, nil
	}

	// Take a screenshot after waiting
	var screenshot []byte
	if err := chromedp.Run(ctx, chromedp.CaptureScreenshot(&screenshot)); err != nil {
		return &BrowseResult{
			Success: true,
			Error:   fmt.Sprintf("wait successful but failed to capture screenshot: %v", err),
			Action:  string(ActionWait),
		}, nil
	}

	// Encode screenshot as base64
	screenshotBase64 := base64.StdEncoding.EncodeToString(screenshot)

	// Save screenshot to file for reference
	screenshotPath := filepath.Join(t.ScreenshotDir, fmt.Sprintf("wait-%d.png", time.Now().UnixNano()))
	os.WriteFile(screenshotPath, screenshot, 0644)

	return &BrowseResult{
		Success:    true,
		Screenshot: screenshotBase64,
		Action:     string(ActionWait),
	}, nil
}

// handleExtractText extracts text from the current page
func (t *WebBrowserTool) handleExtractText(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// Get selector if provided, otherwise use body
	selector, ok := params["selector"].(string)
	if !ok || selector == "" {
		selector = "body"
	}

	// Extract text from the page
	var content string
	var title string
	if err := chromedp.Run(ctx,
		chromedp.Title(&title),
		chromedp.Text(selector, &content, chromedp.ByQuery),
	); err != nil {
		return &BrowseResult{
			Success: false,
			Error:   fmt.Sprintf("failed to extract text: %v", err),
			Action:  string(ActionExtractText),
		}, nil
	}

	// Take a screenshot
	var screenshot []byte
	if err := chromedp.Run(ctx, chromedp.CaptureScreenshot(&screenshot)); err != nil {
		return &BrowseResult{
			Success: true,
			Title:   title,
			Content: content,
			Error:   fmt.Sprintf("text extraction successful but failed to capture screenshot: %v", err),
			Action:  string(ActionExtractText),
		}, nil
	}

	// Encode screenshot as base64
	screenshotBase64 := base64.StdEncoding.EncodeToString(screenshot)

	// Save screenshot to file for reference
	screenshotPath := filepath.Join(t.ScreenshotDir, fmt.Sprintf("extract-%d.png", time.Now().UnixNano()))
	os.WriteFile(screenshotPath, screenshot, 0644)

	return &BrowseResult{
		Success:    true,
		Title:      title,
		Content:    content,
		Screenshot: screenshotBase64,
		Action:     string(ActionExtractText),
	}, nil
}

// closeBrowser closes the browser if it's open
func (t *WebBrowserTool) closeBrowser() {
	// Cancel the browser context if it exists
	if t.allocCancel != nil {
		t.allocCancel()
		t.allocCancel = nil
		t.allocCtx = nil
		t.browserCtx = nil
	}
}

// Cleanup releases browser resources and removes temporary directories
func (t *WebBrowserTool) Cleanup() error {
	// Close the browser if it's open and we're not keeping it open
	if !t.KeepBrowserOpen {
		t.closeBrowser()
	}

	// Clean up temporary directories
	errors := []error{}

	// Remove screenshot directory if it exists and is a temp directory
	if t.ScreenshotDir != "" && strings.Contains(t.ScreenshotDir, "browser-screenshots-") {
		if err := os.RemoveAll(t.ScreenshotDir); err != nil {
			errors = append(errors, fmt.Errorf("failed to remove screenshot directory: %w", err))
		}
	}

	// Remove user data directory if it exists and is a temp directory
	if t.UserDataDir != "" && strings.Contains(t.UserDataDir, "browser-userdata-") {
		if err := os.RemoveAll(t.UserDataDir); err != nil {
			errors = append(errors, fmt.Errorf("failed to remove user data directory: %w", err))
		}
	}

	// Return combined errors if any
	if len(errors) > 0 {
		errMsgs := []string{}
		for _, err := range errors {
			errMsgs = append(errMsgs, err.Error())
		}
		return fmt.Errorf("cleanup errors: %s", strings.Join(errMsgs, "; "))
	}

	return nil
}

// handleNavigate navigates to a URL and takes a screenshot
func (t *WebBrowserTool) handleNavigate(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// Get the URL from parameters
	urlStr, ok := params["url"].(string)
	if !ok || urlStr == "" {
		return nil, fmt.Errorf("url parameter is required and must be a string")
	}

	// Validate the URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	// Ensure the URL has a scheme
	if parsedURL.Scheme == "" {
		urlStr = "https://" + urlStr
		parsedURL, err = url.Parse(urlStr)
		if err != nil {
			return nil, fmt.Errorf("invalid URL: %w", err)
		}
	}

	// Only allow HTTP and HTTPS
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("only HTTP and HTTPS URLs are supported")
	}

	// Variables to store results
	var title string
	var content string
	var screenshot []byte

	// Navigate to the URL and capture screenshot
	err = chromedp.Run(ctx,
		chromedp.Navigate(urlStr),
		chromedp.WaitReady("body", chromedp.ByQuery),
	)

	if err != nil {
		return &BrowseResult{
			Success: false,
			URL:     urlStr,
			Error:   fmt.Sprintf("failed to navigate: %v", err),
			Action:  string(ActionNavigate),
		}, nil
	}

	/* // Get CAPTCHA handling parameters
	useLLM, _ := params["use_llm"].(bool)
	captchaType, _ := params["captcha_type"].(string)

	// Check for and handle CAPTCHA challenges
	captchaDetected, captchaErr := t.detectAndHandleCaptcha(ctx)
	var captchaHandled bool

	if captchaErr != nil {
		// Log the error but continue with navigation
		fmt.Printf("CAPTCHA detection error: %v\n", captchaErr)
	} else if captchaDetected {
		// If CAPTCHA was detected without error, try to handle it
		captchaHandled = true

		// If LLM solving is enabled, use the dedicated CAPTCHA handler
		if useLLM {
			// Create parameters for handleCaptcha
			captchaParams := map[string]interface{}{
				"use_llm": true,
			}

			// Add captcha_type if provided
			if captchaType != "" {
				captchaParams["captcha_type"] = captchaType
			} else {
				captchaParams["captcha_type"] = "generic"
			}

			// Call the handleCaptcha function
			captchaResult, captchaErr := t.handleCaptcha(ctx, captchaParams)
			if captchaErr != nil {
				fmt.Printf("CAPTCHA handling error: %v\n", captchaErr)
			} else if result, ok := captchaResult.(*BrowseResult); ok {
				captchaHandled = result.CaptchaHandled
			}
		}

		// Wait a moment after handling CAPTCHA
		chromedp.Run(ctx, chromedp.Sleep(2*time.Second))
	} */

	// Now capture the page content and screenshot after potential CAPTCHA handling
	err = chromedp.Run(ctx,
		chromedp.Title(&title),
		chromedp.InnerHTML("body", &content, chromedp.ByQuery),
		chromedp.CaptureScreenshot(&screenshot),
	)

	if err != nil {
		return &BrowseResult{
			Success: false,
			URL:     urlStr,
			Error:   fmt.Sprintf("failed to capture page content: %v", err),
			Action:  string(ActionNavigate),
		}, nil
	}

	// Encode screenshot as base64
	screenshotBase64 := base64.StdEncoding.EncodeToString(screenshot)

	// Save screenshot to file for reference
	screenshotPath := filepath.Join(t.ScreenshotDir, fmt.Sprintf("screenshot-%d.png", time.Now().UnixNano()))
	os.WriteFile(screenshotPath, screenshot, 0644)

	// Process content if needed to handle token limits
	var contentTruncated bool
	processedContent := content

	if t.contentProcessor != nil && len(content) > t.contentProcessor.MaxContentLength {
		processedContent = t.contentProcessor.SummarizeContent(content)
		contentTruncated = true
	}

	return &BrowseResult{
		Success:          true,
		URL:              urlStr,
		Title:            title,
		Content:          processedContent,
		Screenshot:       screenshotBase64,
		Action:           string(ActionNavigate),
		ContentTruncated: contentTruncated,
	}, nil
}

// extractHTMLContent extracts the title and main content from HTML
func extractHTMLContent(htmlContent string) (string, string) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return "", htmlContent
	}

	var title string
	var content strings.Builder

	// Extract title
	titleNode := findNode(doc, "title")
	if titleNode != nil {
		title = extractText(titleNode)
	}

	// Extract main content
	// First try to find main content containers
	contentNodes := findContentNodes(doc)
	if len(contentNodes) > 0 {
		for _, node := range contentNodes {
			content.WriteString(extractText(node))
			content.WriteString("\n\n")
		}
	} else {
		// Fallback to extracting all paragraph text
		paragraphs := findAllNodes(doc, "p")
		for _, p := range paragraphs {
			content.WriteString(extractText(p))
			content.WriteString("\n\n")
		}
	}

	return title, content.String()
}

// findNode finds the first occurrence of a node with the given tag name
func findNode(n *html.Node, tagName string) *html.Node {
	if n.Type == html.ElementNode && n.Data == tagName {
		return n
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if result := findNode(c, tagName); result != nil {
			return result
		}
	}

	return nil
}

// findAllNodes finds all nodes with the given tag name
// findContentNodes tries to find main content containers in the HTML
func findContentNodes(n *html.Node) []*html.Node {
	var contentNodes []*html.Node

	// Common content container IDs and classes
	contentIDs := []string{"content", "main", "article", "post"}
	contentClasses := []string{"content", "main", "article", "post", "entry"}

	// Look for nodes with content-related IDs
	for _, id := range contentIDs {
		if node := findNodeByAttribute(n, "id", id); node != nil {
			contentNodes = append(contentNodes, node)
		}
	}

	// Look for nodes with content-related classes
	for _, class := range contentClasses {
		nodes := findNodesByAttribute(n, "class", class)
		contentNodes = append(contentNodes, nodes...)
	}

	// Look for article tags
	articles := findAllNodes(n, "article")
	contentNodes = append(contentNodes, articles...)

	// Look for main tags
	main := findAllNodes(n, "main")
	contentNodes = append(contentNodes, main...)

	return contentNodes
}

// extractText extracts all text from a node and its children
func extractText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}

	var text strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		text.WriteString(extractText(c))
	}

	// Add spacing after block elements
	if n.Type == html.ElementNode {
		switch n.Data {
		case "p", "div", "h1", "h2", "h3", "h4", "h5", "h6", "li":
			text.WriteString("\n")
		}
	}

	return text.String()
}
