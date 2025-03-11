package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/prathyushnallamothu/commandforge/pkg/agent"
	"github.com/prathyushnallamothu/commandforge/pkg/api"
	"github.com/prathyushnallamothu/commandforge/pkg/config"
	"github.com/prathyushnallamothu/commandforge/pkg/executor"
	"github.com/prathyushnallamothu/commandforge/pkg/flow"
	"github.com/prathyushnallamothu/commandforge/pkg/llm"
	"github.com/prathyushnallamothu/commandforge/pkg/memory"
	"github.com/prathyushnallamothu/commandforge/pkg/tools"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "", "Path to config file")
	interactive := flag.Bool("interactive", true, "Run in interactive mode")
	query := flag.String("query", "", "Query to run in non-interactive mode")
	serverMode := flag.Bool("server", false, "Run as API server")
	serverAddr := flag.String("addr", ":8080", "Address for API server to listen on")
	clientMode := flag.Bool("client", false, "Run as API client")
	serverURL := flag.String("server-url", "http://localhost:8080", "URL of the API server")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")
	reactMode := flag.Bool("react", false, "Use ReAct agent instead of standard agent")
	flag.Parse()

	// Enable verbose logging if requested
	if *verbose {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
		log.Println("Verbose logging enabled")
	}

	// Determine config path
	if *configPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("Failed to get user home directory: %v", err)
		}
		*configPath = filepath.Join(homeDir, ".commandforge", "config.json")
	}

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Printf("Warning: Failed to load config: %v", err)
		log.Printf("Using default configuration")
		cfg = config.DefaultConfig()
	}

	// Ensure working directory exists
	if err := os.MkdirAll(cfg.WorkingDir, 0755); err != nil {
		log.Fatalf("Failed to create working directory: %v", err)
	}

	// Check for API key
	apiKey, ok := cfg.APIKeys["openai"]
	if !ok || apiKey == "" {
		// Try environment variable
		apiKey = os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			log.Fatalf("OpenAI API key not found. Please set it in the config file or OPENAI_API_KEY environment variable")
		}
		// Save to config
		if cfg.APIKeys == nil {
			cfg.APIKeys = make(map[string]string)
		}
		cfg.APIKeys["openai"] = apiKey
		config.SaveConfig(cfg, *configPath)
	}

	// Initialize memory
	memoryPath := filepath.Join(cfg.WorkingDir, "memory")
	mem, err := memory.NewFileMemory(memoryPath)
	if err != nil {
		log.Fatalf("Failed to initialize memory: %v", err)
	}

	// Initialize LLM client
	llmClient := llm.NewOpenAIClient(apiKey, "gpt-4o-mini")

	// Create context
	ctx := context.Background()

	// Run the application in the appropriate mode
	if *serverMode {
		// Run as API server
		runServer(ctx, llmClient, mem, cfg.WorkingDir, *serverAddr)
	} else if *clientMode {
		// Run as API client
		runClient(*serverURL, *interactive, *query)
	} else {
		// Run the agent locally
		if *reactMode {
			// Use ReAct agent
			runReActAgent(ctx, llmClient, mem, cfg.WorkingDir, *interactive, *query, cfg)
		} else {
			// Use standard Forge agent
			// Create agent
			forgeAgent := agent.NewForgeAgent("CommandForge", llmClient, mem)

			// Add tools
			addTools(forgeAgent, cfg.WorkingDir, cfg)

			// Initialize agent
			if err := forgeAgent.Initialize(ctx); err != nil {
				log.Fatalf("Failed to initialize agent: %v", err)
			}

			// Run in interactive or query mode
			if *interactive {
				runInteractive(ctx, forgeAgent)
			} else if *query != "" {
				runQuery(ctx, forgeAgent, *query)
			} else {
				fmt.Println("Please provide a query with -query flag or use -interactive mode")
			}
		}
	}
}

// addTools adds tools to the agent
func addTools(forgeAgent *agent.ForgeAgent, workingDir string, cfg *config.Config) {
	// Add bash tool
	bashTool := tools.NewBashTool(workingDir)
	forgeAgent.AddTool(bashTool)

	// Add Python tool
	pythonTool := tools.NewPythonTool(workingDir)
	forgeAgent.AddTool(pythonTool)

	// Add file tool
	fileTool := tools.NewFileTool(workingDir)
	forgeAgent.AddTool(fileTool)

	// Add command status tool
	commandStatusTool := tools.NewCommandStatusTool()
	forgeAgent.AddTool(commandStatusTool)

	// Add list commands tool
	listCommandsTool := tools.NewListCommandsTool()
	forgeAgent.AddTool(listCommandsTool)

	// Add web search tool
	// Check if Tavily API key is available
	if tavilyAPIKey, ok := cfg.APIKeys["tavily"]; ok && tavilyAPIKey != "" {
		// Use Tavily search if API key is available
		webSearchTool := tools.NewWebSearchTool(tavilyAPIKey)
		// Set a higher timeout for web search to ensure it completes
		webSearchTool.WithTimeout(30 * time.Second)
		forgeAgent.AddTool(webSearchTool)
		log.Printf("Added Tavily web search tool with API key")
	} else {
		// Fall back to the version that doesn't require API key
		webSearchTool := tools.NewFallbackWebSearchTool()
		forgeAgent.AddTool(webSearchTool)
		log.Printf("Added fallback web search tool (no Tavily API key found)")
	}

	// Add web browser tool
	webBrowserTool := tools.NewWebBrowserTool()
	forgeAgent.AddTool(webBrowserTool)
}

// runInteractive runs the agent in interactive mode
func runInteractive(ctx context.Context, forgeAgent *agent.ForgeAgent) {
	fmt.Println("Welcome to CommandForge!")
	fmt.Println("Type 'exit' or 'quit' to exit")
	fmt.Println()

	for {
		// Get user input
		fmt.Print("> ")
		var input string
		if _, err := fmt.Scanln(&input); err != nil {
			// Handle empty input
			buffer := make([]byte, 1024)
			n, _ := os.Stdin.Read(buffer)
			input = string(buffer[:n])
			input = strings.TrimSpace(input)
		}

		// Check for exit command
		if input == "exit" || input == "quit" {
			break
		}

		// Skip empty input
		if input == "" {
			continue
		}

		// Process the input
		response, err := forgeAgent.Run(ctx, &agent.Request{Input: input})
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		// Display the response
		if !response.Success {
			fmt.Printf("Error: %s\n", response.Error)
		} else {
			fmt.Println(response.Output)
		}
		fmt.Println()
	}
}

// runQuery runs a single query and exits
func runQuery(ctx context.Context, forgeAgent *agent.ForgeAgent, query string) {
	// Log the query being processed
	log.Printf("Processing query: %s", query)

	// Add a prefix to make the query more autonomous and explicit
	if strings.Contains(strings.ToLower(query), "search") ||
		strings.Contains(strings.ToLower(query), "find") ||
		strings.Contains(strings.ToLower(query), "look up") ||
		strings.Contains(strings.ToLower(query), "information about") {
		// For search queries, make them more explicit
		query = fmt.Sprintf("Use the web_search tool to %s. Then analyze the results and provide a comprehensive summary of your findings.", query)
	} else {
		// For other queries, make them more autonomous
		query = fmt.Sprintf("Act as an autonomous agent and %s. Use all available tools as needed to provide a comprehensive response.", query)
	}

	log.Printf("Enhanced query: %s", query)

	// Process the query
	response, err := forgeAgent.Run(ctx, &agent.Request{Input: query})
	if err != nil {
		log.Fatalf("Error running query: %v", err)
	}

	// Display the response
	log.Printf("Response received: success=%v, has output=%v, has error=%v",
		response.Success, response.Output != "", response.Error != "")

	if !response.Success {
		fmt.Printf("Error: %s\n", response.Error)
		os.Exit(1)
	} else {
		// Ensure we have output to display
		if response.Output == "" {
			// Log detailed response information for debugging
			log.Printf("Warning: Empty output received from agent")

			// Access conversation history directly for debugging
			history := forgeAgent.ConversationHistory
			log.Printf("Conversation history length: %d", len(history))

			// Log the last few messages for context
			startIdx := len(history) - 5
			if startIdx < 0 {
				startIdx = 0
			}

			log.Printf("Last few conversation messages:")
			for i := startIdx; i < len(history); i++ {
				msg := history[i]
				log.Printf("  [%s] Content length: %d", msg.Role, len(msg.Content))
			}

			fmt.Println("No output received from the agent. Please try a more specific query or check the logs for details.")
		} else {
			fmt.Println(response.Output)
		}
	}
}

// runServer runs the application as an API server
func runServer(ctx context.Context, llmClient llm.Client, mem agent.Memory, workingDir, addr string) {
	// Create agent factory
	agentFactory := agent.NewFactory(llmClient, mem)

	// Create flow factory
	flowFactory := flow.NewFlowFactory(llmClient, mem, agentFactory)

	// Create flow manager
	flowManager := flow.NewFlowManager(flowFactory)

	// Create API server
	server := api.NewServer(addr, flowManager)

	// Start server
	log.Printf("Starting API server on %s\n", addr)
	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// runClient runs the application as an API client
func runClient(serverURL string, interactive bool, query string) {
	// Create API client
	client := api.NewClient(serverURL)

	// Run in the appropriate mode
	if interactive {
		runInteractiveClient(client)
	} else if query != "" {
		runQueryClient(client, query)
	} else {
		fmt.Println("Please provide a query with -query flag or use -interactive mode")
	}
}

// runInteractiveClient runs the client in interactive mode
func runInteractiveClient(client *api.Client) {
	// Print welcome message
	fmt.Println("Welcome to CommandForge API Client!")
	fmt.Println("Type 'exit' or 'quit' to exit")
	fmt.Println()

	// Create a scanner for reading input
	scanner := bufio.NewScanner(os.Stdin)

	// Create a flow
	fmt.Print("Enter a goal for the flow: ")
	if !scanner.Scan() {
		return
	}
	goal := scanner.Text()

	flowID, err := client.CreateFlow("planning", goal)
	if err != nil {
		fmt.Printf("Failed to create flow: %v\n", err)
		return
	}

	fmt.Printf("Created flow with ID: %s\n", flowID)
	fmt.Println()

	// Main input loop
	for {
		// Get user input
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		input := scanner.Text()

		// Check for exit command
		if input == "exit" || input == "quit" {
			break
		}

		// Skip empty input
		if input == "" {
			continue
		}

		// Execute command
		commandID, err := client.ExecuteCommand(flowID, input)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		fmt.Printf("Executing command (ID: %s)...\n", commandID)

		// Stream command status
		err = client.StreamCommandStatus(flowID, commandID, func(status *executor.BackgroundCommandStatus) {
			// Display command status
			displayCommandStatus(status)
		})

		if err != nil {
			fmt.Printf("Error streaming command status: %v\n", err)

			// Fall back to polling
			for {
				status, err := client.GetCommandStatus(flowID, commandID)
				if err != nil {
					fmt.Printf("Error getting command status: %v\n", err)
					break
				}

				// Display command status
				displayCommandStatus(status)

				// If command is no longer running, break
				if !status.Running {
					break
				}

				// Wait before polling again
				time.Sleep(500 * time.Millisecond)
			}
		}

		fmt.Println()
	}
}

// runQueryClient runs a single query and exits
func runQueryClient(client *api.Client, query string) {
	// Create a flow
	flowID, err := client.CreateFlow("planning", "Execute a command")
	if err != nil {
		log.Fatalf("Failed to create flow: %v", err)
	}

	// Execute command
	commandID, err := client.ExecuteCommand(flowID, query)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	fmt.Printf("Executing command (ID: %s)...\n", commandID)

	// Poll for command status
	for {
		status, err := client.GetCommandStatus(flowID, commandID)
		if err != nil {
			log.Fatalf("Error getting command status: %v", err)
		}

		// Display command status
		displayCommandStatus(status)

		// If command is no longer running, break
		if !status.Running {
			break
		}

		// Wait before polling again
		time.Sleep(500 * time.Millisecond)
	}
}

// displayCommandStatus displays the status of a command
func displayCommandStatus(status *executor.BackgroundCommandStatus) {
	// Clear screen (optional)
	// fmt.Print("\033[H\033[2J")

	// Display status
	statusColor := color.New(color.FgYellow)
	if !status.Running {
		if status.ExitCode == 0 {
			statusColor = color.New(color.FgGreen)
		} else {
			statusColor = color.New(color.FgRed)
		}
	}

	// Status line
	statusText := "Running"
	if !status.Running {
		if status.ExitCode == 0 {
			statusText = "Completed successfully"
		} else {
			statusText = fmt.Sprintf("Failed with exit code %d", status.ExitCode)
		}
	}

	statusColor.Printf("Status: %s (%.2f seconds)\n", statusText, status.Duration)

	// Output
	outputColor := color.New(color.FgCyan)
	if len(status.OutputList) > 0 {
		outputColor.Println("Output:")
		for _, line := range status.OutputList {
			fmt.Println(line)
		}
	}

	// Error
	errorColor := color.New(color.FgRed)
	if len(status.ErrorList) > 0 {
		errorColor.Println("Error:")
		for _, line := range status.ErrorList {
			fmt.Println(line)
		}
	}
}
