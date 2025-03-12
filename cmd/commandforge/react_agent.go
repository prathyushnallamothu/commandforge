package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/prathyushnallamothu/commandforge/pkg/agent"
	"github.com/prathyushnallamothu/commandforge/pkg/config"
	"github.com/prathyushnallamothu/commandforge/pkg/llm"
	"github.com/prathyushnallamothu/commandforge/pkg/tools"
)

// runReActAgent runs the application with the ReAct agent
func runReActAgent(ctx context.Context, llmClient llm.Client, mem agent.Memory, workingDir string, interactive bool, query string, cfg *config.Config) {
	// Create ReAct agent
	reactAgent := agent.NewReActAgent("CommandForge", llmClient, mem)

	// Add tools
	addReActTools(reactAgent, workingDir, cfg)

	// Initialize agent
	if err := reactAgent.Initialize(ctx); err != nil {
		log.Fatalf("Failed to initialize ReAct agent: %v", err)
	}

	// Run in interactive or query mode
	if interactive {
		runReActInteractive(ctx, reactAgent)
	} else {
		runReActQuery(ctx, reactAgent, query)
	}
}

// runReActInteractive runs the ReAct agent in interactive mode
func runReActInteractive(ctx context.Context, reactAgent *agent.ReActAgent) {
	// Print welcome message
	fmt.Println("Welcome to CommandForge (ReAct Mode)!")
	fmt.Println("Type 'exit' to quit.")
	fmt.Println()

	// Create a scanner for user input
	scanner := bufio.NewScanner(os.Stdin)

	// Main interaction loop
	for {
		// Print prompt
		fmt.Print(color.GreenString("CommandForge> "))

		// Get user input
		if !scanner.Scan() {
			break
		}
		input := scanner.Text()

		// Check for exit command
		if strings.ToLower(input) == "exit" {
			break
		}

		// Skip empty input
		if input == "" {
			continue
		}

		// Create request
		request := &agent.Request{
			Input: input,
		}

		// Process request
		response, err := reactAgent.Run(ctx, request)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		// Check for error in response
		if !response.Success {
			fmt.Printf("Error: %s\n", response.Error)
			continue
		}

		// Print response
		fmt.Println()
		fmt.Println(response.Output)
		fmt.Println()
	}
}

// runReActQuery runs a single query with the ReAct agent and exits
func runReActQuery(ctx context.Context, reactAgent *agent.ReActAgent, query string) {
	// Create request
	request := &agent.Request{
		Input: query,
	}

	// Process request
	response, err := reactAgent.Run(ctx, request)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Check for error in response
	if !response.Success {
		fmt.Printf("Error: %s\n", response.Error)
		os.Exit(1)
	}

	// Print response
	fmt.Println(response.Output)
}

// addReActTools adds tools to the ReAct agent
func addReActTools(reactAgent *agent.ReActAgent, workingDir string, cfg *config.Config) {
	// Add bash tool
	bashTool := tools.NewBashTool(workingDir)
	reactAgent.AddTool(bashTool)

	// Add Python tool
	pythonTool := tools.NewPythonTool(workingDir)
	reactAgent.AddTool(pythonTool)

	// Add file tool
	fileTool := tools.NewFileTool(workingDir)
	reactAgent.AddTool(fileTool)

	// Add command status tool
	commandStatusTool := tools.NewCommandStatusTool()
	reactAgent.AddTool(commandStatusTool)

	// Add list commands tool
	listCommandsTool := tools.NewListCommandsTool()
	reactAgent.AddTool(listCommandsTool)

	// Add web search tool
	// Check if Tavily API key is available
	if tavilyAPIKey, ok := cfg.APIKeys["tavily"]; ok && tavilyAPIKey != "" {
		// Use Tavily search if API key is available
		webSearchTool := tools.NewWebSearchTool(tavilyAPIKey)
		// Set a higher timeout for web search to ensure it completes
		webSearchTool.WithTimeout(30 * time.Second)
		reactAgent.AddTool(webSearchTool)
	} else {
		// Fall back to the version that doesn't require API key
		webSearchTool := tools.NewFallbackWebSearchTool()
		reactAgent.AddTool(webSearchTool)
	}

	// Add web browser tool
	webBrowserTool := tools.NewWebBrowserTool()
	reactAgent.AddTool(webBrowserTool)
}
