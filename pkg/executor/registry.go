package executor

import (
	"fmt"
	"sync"
)

// CommandRegistry is a registry for background commands
var commandRegistry = struct {
	mu       sync.RWMutex
	commands map[string]*BackgroundCommand
}{
	commands: make(map[string]*BackgroundCommand),
}

// RegisterCommand adds a command to the registry
func RegisterCommand(cmd *BackgroundCommand) {
	commandRegistry.mu.Lock()
	defer commandRegistry.mu.Unlock()
	commandRegistry.commands[cmd.ID] = cmd
}

// GetCommand retrieves a command from the registry
func GetCommand(id string) (*BackgroundCommand, error) {
	commandRegistry.mu.RLock()
	defer commandRegistry.mu.RUnlock()
	cmd, ok := commandRegistry.commands[id]
	if !ok {
		return nil, fmt.Errorf("command not found: %s", id)
	}
	return cmd, nil
}

// ListCommands returns a list of all command IDs
func ListCommands() []string {
	commandRegistry.mu.RLock()
	defer commandRegistry.mu.RUnlock()
	ids := make([]string, 0, len(commandRegistry.commands))
	for id := range commandRegistry.commands {
		ids = append(ids, id)
	}
	return ids
}

// RemoveCommand removes a command from the registry
func RemoveCommand(id string) {
	commandRegistry.mu.Lock()
	defer commandRegistry.mu.Unlock()
	delete(commandRegistry.commands, id)
}
