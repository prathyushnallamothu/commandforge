package tools

import (
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// ContentProcessor handles processing and truncating browser content
// to prevent token limit issues with LLMs and manages CAPTCHA detection
type ContentProcessor struct {
	MaxContentLength int // Maximum length of content in characters
	MaxTitleLength   int // Maximum length of title in characters
	CaptchaKeywords  []string // Keywords that might indicate a CAPTCHA
	CaptchaSelectors []string // CSS selectors that might indicate a CAPTCHA
}

// NewContentProcessor creates a new content processor with default settings
func NewContentProcessor() *ContentProcessor {
	return &ContentProcessor{
		MaxContentLength: 8000, // Default max content length
		MaxTitleLength:   200,  // Default max title length
		CaptchaKeywords: []string{
			"captcha", "robot", "human verification", "security check",
			"prove you're human", "bot check", "verification", "not a robot",
			"human test", "challenge", "security verification",
		},
		CaptchaSelectors: []string{
			".g-recaptcha", "iframe[src*=\"recaptcha\"]", "iframe[src*=\"google.com/recaptcha\"]",
			".h-captcha", "iframe[src*=\"hcaptcha\"]",
			"div[class*=\"captcha\"]", "img[src*=\"captcha\"]", "input[name*=\"captcha\"]",
		},
	}
}

// ProcessBrowseResult processes a browse result to ensure it doesn't exceed token limits
// and enhances it with CAPTCHA detection information
func (p *ContentProcessor) ProcessBrowseResult(result *BrowseResult) *BrowseResult {
	// Create a copy of the result to avoid modifying the original
	processedResult := *result

	// Truncate title if needed
	if len(result.Title) > p.MaxTitleLength {
		processedResult.Title = result.Title[:p.MaxTitleLength] + "... (truncated)"
	}

	// Process content if it exists and is too large
	if len(result.Content) > p.MaxContentLength {
		processedResult.Content = p.SummarizeContent(result.Content)
		processedResult.ContentTruncated = true
	}

	// Process screenshot if needed (optionally downsample or remove)
	if len(result.Screenshot) > 100000 { // If screenshot is very large
		// Option 1: Remove screenshot entirely if it's extremely large
		// processedResult.Screenshot = ""

		// Option 2: Include a note that screenshot was removed
		processedResult.Screenshot = "[Screenshot removed due to size constraints]"
	}

	return &processedResult
}

// SummarizeContent intelligently summarizes content to fit within token limits
func (p *ContentProcessor) SummarizeContent(content string) string {
	// If content is HTML, try to extract and summarize the main text
	if strings.Contains(content, "<html") || strings.Contains(content, "<body") {
		return p.SummarizeHTML(content)
	}

	// For plain text, extract important sections
	return p.SummarizeText(content)
}

// SummarizeHTML extracts and summarizes the main content from HTML
func (p *ContentProcessor) SummarizeHTML(htmlContent string) string {
	// Parse the HTML
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		// If parsing fails, fall back to text summarization
		return p.SummarizeText(htmlContent)
	}

	// Extract main content
	var sb strings.Builder

	// Try to find and extract content from main content areas
	contentNodes := findContentNodes(doc)
	if len(contentNodes) > 0 {
		for _, node := range contentNodes {
			sb.WriteString(extractText(node))
			sb.WriteString("\n\n")
		}
	} else {
		// Extract headings and paragraphs
		headings := []string{"h1", "h2", "h3"}
		for _, tag := range headings {
			nodes := findAllNodes(doc, tag)
			for _, node := range nodes {
				sb.WriteString(extractText(node))
				sb.WriteString("\n")
			}
		}

		// Add some paragraphs
		paragraphs := findAllNodes(doc, "p")
		pCount := 0
		for _, node := range paragraphs {
			text := extractText(node)
			if len(text) > 20 { // Only include substantial paragraphs
				sb.WriteString(text)
				sb.WriteString("\n\n")
				pCount++
				if pCount >= 10 { // Limit number of paragraphs
					break
				}
			}
		}
	}

	// Get the extracted content
	extractedContent := sb.String()

	// If still too long, truncate with intelligent breaks
	if utf8.RuneCountInString(extractedContent) > p.MaxContentLength {
		return p.TruncateWithSummary(extractedContent)
	}

	return extractedContent
}

// SummarizeText summarizes plain text content
func (p *ContentProcessor) SummarizeText(text string) string {
	// Split into paragraphs
	paragraphs := strings.Split(text, "\n\n")

	// If we have a reasonable number of paragraphs, keep important ones
	if len(paragraphs) > 5 {
		// Keep first 2 paragraphs
		start := paragraphs[:2]

		// Keep last 2 paragraphs
		end := paragraphs[len(paragraphs)-2:]

		// Add a note about truncation
		middle := []string{"[...content truncated...]"}

		// Combine the parts
		summarized := append(start, middle...)
		summarized = append(summarized, end...)

		result := strings.Join(summarized, "\n\n")

		// If still too long, perform final truncation
		if utf8.RuneCountInString(result) > p.MaxContentLength {
			return p.TruncateWithSummary(result)
		}

		return result
	}

	// If content is still too long, truncate directly
	return p.TruncateWithSummary(text)
}

// TruncateWithSummary truncates text to the maximum length with a summary note
func (p *ContentProcessor) TruncateWithSummary(text string) string {
	// Calculate safe truncation point (at word boundary if possible)
	maxLen := p.MaxContentLength - 50 // Leave room for truncation message
	truncateAt := maxLen

	// Find a good breaking point (end of sentence or paragraph)
	goodBreaks := []string{".", "!", "?", "\n"}
	for _, breakChar := range goodBreaks {
		lastBreak := strings.LastIndex(text[:maxLen], breakChar)
		if lastBreak > maxLen/2 { // Only use if it's not too early in the text
			truncateAt = lastBreak + 1 // Include the break character
			break
		}
	}

	// Get beginning of text
	beginning := text[:truncateAt]

	// Get a small sample from the end
	endSampleSize := 500
	endSample := ""
	if len(text) > endSampleSize {
		endSample = "\n\n[...content truncated...]\n\n" + text[len(text)-endSampleSize:]
	}

	// Combine with truncation notice
	return beginning + "\n\n[Content truncated due to length...]" + endSample
}

// DecodeAndResizeScreenshot decodes a base64 screenshot and optionally resizes it
func (p *ContentProcessor) DecodeAndResizeScreenshot(base64Screenshot string) ([]byte, error) {
	// Decode the base64 string
	screenshot, err := base64.StdEncoding.DecodeString(base64Screenshot)
	if err != nil {
		return nil, fmt.Errorf("failed to decode screenshot: %w", err)
	}

	// For now, just return the decoded screenshot
	// In a full implementation, you could resize the image here
	return screenshot, nil
}

// DetectCaptchaInContent analyzes HTML content to detect CAPTCHA challenges
func (p *ContentProcessor) DetectCaptchaInContent(content string) (bool, string) {
	// Check for CAPTCHA keywords in the content
	lowerContent := strings.ToLower(content)
	for _, keyword := range p.CaptchaKeywords {
		if strings.Contains(lowerContent, strings.ToLower(keyword)) {
			return true, fmt.Sprintf("Detected CAPTCHA keyword: %s", keyword)
		}
	}

	// Parse the HTML to check for CAPTCHA selectors
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		// If we can't parse the HTML, just return false
		return false, ""
	}

	// Check for common CAPTCHA elements
	for _, selector := range p.CaptchaSelectors {
		// For simple element selectors (like .classname or #id)
		if strings.HasPrefix(selector, ".") {
			// Class selector
			className := selector[1:]
			nodes := findNodesByAttribute(doc, "class", className)
			if len(nodes) > 0 {
				return true, fmt.Sprintf("Detected CAPTCHA element with class: %s", className)
			}
		} else if strings.HasPrefix(selector, "#") {
			// ID selector
			id := selector[1:]
			node := findNodeByAttribute(doc, "id", id)
			if node != nil {
				return true, fmt.Sprintf("Detected CAPTCHA element with id: %s", id)
			}
		} else if strings.Contains(selector, "[src*=\"") {
			// Partial src attribute selector (like iframe[src*="recaptcha"])
			parts := strings.Split(selector, "[src*=\"")
			if len(parts) == 2 {
				tagName := parts[0]
				srcValue := strings.TrimSuffix(parts[1], "]\"")
				nodes := findAllNodes(doc, tagName)
				for _, node := range nodes {
					for i := 0; i < len(node.Attr); i++ {
						if node.Attr[i].Key == "src" && strings.Contains(node.Attr[i].Val, srcValue) {
							return true, fmt.Sprintf("Detected CAPTCHA %s with src containing: %s", tagName, srcValue)
						}
					}
				}
			}
		}
	}

	return false, ""
}

// EnhanceBrowseResultWithCaptchaInfo adds CAPTCHA detection information to the result content
func (p *ContentProcessor) EnhanceBrowseResultWithCaptchaInfo(result *BrowseResult) *BrowseResult {
	if result == nil {
		return nil
	}

	return result
}


