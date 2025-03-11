package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/chromedp/chromedp"
)

// handleDialog handles common dialog patterns like cookie consent, popups, etc.
func (t *WebBrowserTool) handleDialog(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// Convert map to struct
	browserParams, err := paramsToStruct(params)
	if err != nil {
		return nil, err
	}

	// Get the dialog type
	dialogType := browserParams.DialogType
	if dialogType == "" {
		dialogType = "cookie" // Default to cookie consent dialog
	}

	// Get the URL if provided
	urlStr, _ := params["url"].(string)
	if urlStr != "" {
		// Navigate to the URL first
		navResult, err := t.handleNavigate(ctx, map[string]interface{}{"url": urlStr})
		if err != nil {
			return nil, err
		}
		
		// Check if navigation was successful
		result, ok := navResult.(*BrowseResult)
		if !ok || !result.Success {
			return result, nil
		}
	}

	// Variables to store results
	var title string
	var content string
	var screenshot []byte

	// Handle different dialog types
	switch dialogType {
	case "cookie":
		// Common selectors for cookie consent dialogs
		cookieSelectors := []string{
			"button[id*='accept']", 
			"button[id*='cookie']", 
			"button[class*='accept']", 
			"button[class*='cookie']",
			"button[aria-label*='Accept']",
			"button[data-testid*='cookie']",
			"a[id*='accept']",
			"a[class*='accept']",
			"div[id*='accept']",
			"div[class*='accept']",
			"#accept-cookies",
			".accept-cookies",
			"#acceptCookies",
			".acceptCookies",
			"#accept_cookies",
			".accept_cookies",
			"#acceptAllCookies",
			".acceptAllCookies",
		}

		// Try each selector
		for _, selector := range cookieSelectors {
			err := chromedp.Run(ctx,
				chromedp.Evaluate(`
					function clickIfVisible(selector) {
						const element = document.querySelector(selector);
						if (element && element.offsetParent !== null) {
							element.click();
							return true;
						}
						return false;
					}
					return clickIfVisible(`+"\""+ selector +"\""+ `);
				`, nil),
				chromedp.Sleep(500*time.Millisecond),
			)
			
			if err == nil {
				// Wait a moment for the dialog to disappear
				chromedp.Run(ctx, chromedp.Sleep(1*time.Second))
				break
			}
		}

	case "popup":
		// Common selectors for popup dialogs
		popupSelectors := []string{
			"button[class*='close']",
			"div[class*='close']",
			"span[class*='close']",
			"button[aria-label*='Close']",
			"button[data-testid*='close']",
			".modal-close",
			"#modal-close",
			".popup-close",
			"#popup-close",
		}

		// Try each selector
		for _, selector := range popupSelectors {
			err := chromedp.Run(ctx,
				chromedp.Evaluate(`
					function clickIfVisible(selector) {
						const element = document.querySelector(selector);
						if (element && element.offsetParent !== null) {
							element.click();
							return true;
						}
						return false;
					}
					return clickIfVisible(`+"\""+ selector +"\""+ `);
				`, nil),
				chromedp.Sleep(500*time.Millisecond),
			)
			
			if err == nil {
				// Wait a moment for the dialog to disappear
				chromedp.Run(ctx, chromedp.Sleep(1*time.Second))
				break
			}
		}

	default:
		return &BrowseResult{
			Success: false,
			Error:   fmt.Sprintf("unsupported dialog type: %s", dialogType),
			Action:  string(ActionHandleDialog),
		}, nil
	}

	// Capture the current state after handling the dialog
	err = chromedp.Run(ctx,
		chromedp.Title(&title),
		chromedp.InnerHTML("body", &content, chromedp.ByQuery),
		chromedp.CaptureScreenshot(&screenshot),
	)

	if err != nil {
		return &BrowseResult{
			Success: false,
			Error:   fmt.Sprintf("failed to capture page state: %v", err),
			Action:  string(ActionHandleDialog),
		}, nil
	}

	// Encode screenshot as base64
	screenshotBase64 := base64.StdEncoding.EncodeToString(screenshot)

	// Save screenshot to file for reference
	screenshotPath := filepath.Join(t.ScreenshotDir, fmt.Sprintf("dialog-%d.png", time.Now().UnixNano()))
	os.WriteFile(screenshotPath, screenshot, 0644)

	return &BrowseResult{
		Success:    true,
		Title:      title,
		Content:    content,
		Screenshot: screenshotBase64,
		Action:     string(ActionHandleDialog),
	}, nil
}

// detectAndHandleCaptcha attempts to detect and handle CAPTCHA challenges during navigation
func (t *WebBrowserTool) detectAndHandleCaptcha(ctx context.Context) (bool, error) {
	// Variables to track CAPTCHA detection
	var captchaDetected bool

	// Check for common CAPTCHA indicators
	err := chromedp.Run(ctx, chromedp.Evaluate(`
		function detectCaptcha() {
			// Check for reCAPTCHA
			const recaptchaPresent = !!document.querySelector('.g-recaptcha') || 
				!!document.querySelector('iframe[src*="recaptcha"]') ||
				!!document.querySelector('iframe[src*="google.com/recaptcha"]');
			
			// Check for hCaptcha
			const hcaptchaPresent = !!document.querySelector('.h-captcha') || 
				!!document.querySelector('iframe[src*="hcaptcha"]');
			
			// Check for generic CAPTCHA keywords
			const pageText = document.body.innerText.toLowerCase();
			const captchaKeywords = ['captcha', 'robot', 'human verification', 'security check',
				'prove you\'re human', 'bot check', 'verification'];
			
			let keywordPresent = false;
			for (let keyword of captchaKeywords) {
				if (pageText.includes(keyword)) {
					keywordPresent = true;
					break;
				}
			}
			
			return recaptchaPresent || hcaptchaPresent || keywordPresent;
		}
		return detectCaptcha();
	`, &captchaDetected))

	if err != nil {
		return false, err
	}

	if captchaDetected {
		// Try to handle the CAPTCHA by clicking the checkbox
		var captchaHandled bool
		err = chromedp.Run(ctx, chromedp.Evaluate(`
			function handleCaptcha() {
				// Try to find and click reCAPTCHA checkbox
				const recaptchaFrames = document.querySelectorAll('iframe[src*="recaptcha"]');
				for (let frame of recaptchaFrames) {
					try {
						const checkbox = frame.contentDocument.querySelector('.recaptcha-checkbox');
						if (checkbox) {
							checkbox.click();
							return true;
						}
					} catch (e) {
						// Cross-origin restrictions may prevent this
					}
				}
				
				// Try to find and click hCaptcha checkbox
				const hcaptchaFrames = document.querySelectorAll('iframe[src*="hcaptcha"]');
				for (let frame of hcaptchaFrames) {
					try {
						const checkbox = frame.contentDocument.querySelector('.checkbox');
						if (checkbox) {
							checkbox.click();
							return true;
						}
					} catch (e) {
						// Cross-origin restrictions may prevent this
					}
				}
				
				// Try clicking directly on visible CAPTCHA elements
				const captchaElements = [
					document.querySelector('.g-recaptcha'),
					document.querySelector('.h-captcha'),
					document.querySelector('[id*="captcha"]'),
					document.querySelector('[class*="captcha"]')
				];
				
				for (let element of captchaElements) {
					if (element && element.offsetParent !== null) {
						element.click();
						return true;
					}
				}
				
				return false;
			}
			return handleCaptcha();
		`, &captchaHandled))
		
		// Wait a moment after attempting to handle the CAPTCHA
		if err == nil && captchaHandled {
			chromedp.Run(ctx, chromedp.Sleep(2*time.Second))
		}
		
		return true, nil
	}

	return false, nil
}

// handleMultiStep handles multiple browser actions in sequence
func (t *WebBrowserTool) handleMultiStep(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// Convert map to struct
	browserParams, err := paramsToStruct(params)
	if err != nil {
		return nil, err
	}

	// Check if steps are provided
	if browserParams.Steps == nil || len(browserParams.Steps) == 0 {
		return &BrowseResult{
			Success: false,
			Error:   "no steps provided for multi-step action",
			Action:  string(ActionMultiStep),
		}, nil
	}

	// Execute each step in sequence
	var results []*BrowseResult
	var lastResult interface{}
	var lastError error

	for i, step := range browserParams.Steps {
		// Convert step to map
		stepMap := make(map[string]interface{})
		stepBytes, _ := json.Marshal(step)
		json.Unmarshal(stepBytes, &stepMap)

		// Execute the step
		stepResult, err := t.Execute(ctx, stepMap)
		if err != nil {
			return &BrowseResult{
				Success: false,
				Error:   fmt.Sprintf("failed at step %d: %v", i+1, err),
				Action:  string(ActionMultiStep),
			}, nil
		}

		// Check if the step was successful
		result, ok := stepResult.(*BrowseResult)
		if !ok || !result.Success {
			// Check if we should retry
			if browserParams.Retries > 0 {
				// Retry the step
				for retry := 0; retry < browserParams.Retries; retry++ {
					// Wait before retrying
					time.Sleep(time.Duration(browserParams.WaitTime*1000) * time.Millisecond)

					// Execute the step again
					stepResult, err = t.Execute(ctx, stepMap)
					if err == nil {
						result, ok = stepResult.(*BrowseResult)
						if ok && result.Success {
							break
						}
					}
				}

				// If still not successful after retries, return the error
				if !ok || !result.Success {
					return &BrowseResult{
						Success: false,
						Error:   fmt.Sprintf("failed at step %d after %d retries: %v", i+1, browserParams.Retries, result.Error),
						Action:  string(ActionMultiStep),
					}, nil
				}
			} else {
				// No retries, return the error
				return &BrowseResult{
					Success: false,
					Error:   fmt.Sprintf("failed at step %d: %v", i+1, result.Error),
					Action:  string(ActionMultiStep),
				}, nil
			}
		}

		// Add the result to the list
		results = append(results, result)
		lastResult = stepResult
		lastError = err

		// Wait between steps if specified
		if browserParams.WaitTime > 0 {
			time.Sleep(time.Duration(browserParams.WaitTime*1000) * time.Millisecond)
		}
	}

	// Return the last result
	if lastResult != nil {
		result, ok := lastResult.(*BrowseResult)
		if ok {
			// Add multi-step action info
			result.Action = string(ActionMultiStep)
			return result, nil
		}
	}

	// Fallback if something went wrong
	return &BrowseResult{
		Success: false,
		Error:   fmt.Sprintf("failed to execute multi-step action: %v", lastError),
		Action:  string(ActionMultiStep),
	}, nil
}
