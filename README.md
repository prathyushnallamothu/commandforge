# CommandForge

CommandForge is an autonomous AI agent framework built in Go that empowers developers to create powerful, tool-using AI agents that can execute commands, browse the web, and perform complex tasks with minimal human intervention.

## Features

- **Autonomous Operation**: Agents can plan and execute multi-step tasks autonomously
- **Multiple Agent Types**: ReAct agents for reasoning, ToolCall agents for execution, and ForgeAgents combining both
- **Tool Ecosystem**: Pre-built tools for bash commands, Python execution, file management, web search, and web browsing
- **Flexible Architecture**: Modular design allows easy extension with new tools and capabilities
- **Background Command Execution**: Execute commands with real-time streaming output
- **Planning Capabilities**: Break down complex tasks into executable steps
- **Web UI and API Server**: Control agents through a web interface or API

## Installation

### Prerequisites

- Go 1.24+
- OpenAI API key (for GPT-4o models)
- DeepSeek API key (for deepseek-chat models)
- Optional: Tavily API key (for enhanced web search capabilities)

### Quick Start

1. Clone the repository:

   ```bash
   git clone https://github.com/prathyushnallamothu/commandforge.git
   cd commandforge
   ```

2. Copy the example environment file and edit it with your API keys:

   ```bash
   cp .env.example .env
   ```

3. Build the application:

   ```bash
   go build ./cmd/commandforge
   ```

4. Run CommandForge in interactive mode:
   ```bash
   ./commandforge -interactive
   ```

## Configuration

CommandForge looks for a configuration file at `~/.commandforge/config.json`. You can specify a different path using the `-config` flag.

Example configuration:

```json
{
  "llm_provider": "openai",
  "api_keys": {
    "tavily": "YOUR_TAVILY_API_KEY_HERE",
    "openai": "YOUR_OPENAI_API_KEY_HERE"
  },
  "log_level": "debug",
  "working_dir": "/path/to/working/directory",
  "max_memory_size": 100,
  "timeout_seconds": 60
}
```

## Usage

### Command Line Interface

Run CommandForge in interactive mode:

```bash
./commandforge -interactive
```

Run a single query:

```bash
./commandforge -query "Create a Python script that generates the Fibonacci sequence"
```

Run as an API server:

```bash
./commandforge -server -addr ":8080"
```

Connect to a running API server:

```bash
./commandforge -client -server-url "http://localhost:8080"
```

### Examples

Here are some examples of tasks you can ask CommandForge to perform:

```
> Research climate change impacts and create a summary report
> Find the latest news about artificial intelligence and save it to a file
> Analyze the system's CPU usage and display it as a graph
> Create a simple web server in Python
> Search for information about electric vehicles and summarize the findings
```

## Architecture

CommandForge is built with a modular architecture that consists of several key components:

### Agents

- **BaseAgent**: Provides common functionality for all agents
- **ForgeAgent**: The main agent combining reasoning and tool calling
- **ReActAgent**: Agent focused on reasoning using the ReAct pattern
- **ToolCallAgent**: Agent specialized for executing tools

### Tools

- **BashTool**: Execute bash commands
- **PythonTool**: Execute Python code
- **FileTool**: Manage files and directories
- **WebSearchTool**: Search the web for information
- **WebBrowserTool**: Browse web pages and interact with them

### Flows

- **PlanningFlow**: Breaks down complex tasks into a series of steps
- **SimpleFlow**: Basic flow for direct LLM interaction

## Extending CommandForge

### Adding a New Tool

1. Create a new Go file in the `pkg/tools` directory
2. Implement the `Tool` interface
3. Register the tool with the agent in `cmd/commandforge/main.go`

Example of a simple tool implementation:

```go
package tools

import (
	"context"
	"fmt"
)

// MyTool provides custom functionality
type MyTool struct {
	*BaseTool
}

// NewMyTool creates a new instance of MyTool
func NewMyTool() *MyTool {
	return &MyTool{
		BaseTool: NewBaseTool(
			"my_tool",
			"Description of my custom tool",
		),
	}
}

// Execute implements the Tool interface
func (t *MyTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// Implement your tool logic here
	return map[string]interface{}{
		"result": "Tool execution succeeded",
	}, nil
}
```

## API Server

CommandForge can run as an API server, allowing you to interact with it programmatically.

### Endpoints

- `POST /api/v1/flows`: Create a new flow
- `GET /api/v1/flows/{id}`: Get information about a flow
- `POST /api/v1/flows/{id}/execute`: Execute a command in a flow
- `GET /api/v1/flows/{id}/commands/{command_id}`: Get the status of a command
- `GET /api/v1/flows/{id}/commands/{command_id}/stream`: Stream command updates via WebSocket

## Advanced Features

### Memory Management

CommandForge includes a persistent memory system that allows agents to store and retrieve information across sessions:

```go
// Save to memory
agent.SaveMemory(ctx, "key", value)

// Load from memory
value, err := agent.LoadMemory(ctx, "key")
```

### Multi-step Planning

For complex tasks, the planning flow can break down the task into manageable steps:

```
> Plan and execute: Create a Python web scraper for news articles, then analyze the sentiment of the articles
```

### Background Command Execution

Commands can be executed in the background with real-time streaming output:

```go
commandID, err := pipeline.ExecuteCommandInBackground("long-running-command")
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Acknowledgements

- OpenAI for GPT models
- Tavily for web search capabilities
- The Go programming language and its ecosystem

## Support

If you encounter any issues or have questions, please file an issue on the GitHub repository.
