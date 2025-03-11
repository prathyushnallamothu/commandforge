package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds the application configuration
type Config struct {
	LLMProvider   string            `json:"llm_provider"`
	APIKeys       map[string]string `json:"api_keys"`
	LogLevel      string            `json:"log_level"`
	WorkingDir    string            `json:"working_dir"`
	MaxMemorySize int               `json:"max_memory_size"`
	Timeout       int               `json:"timeout_seconds"`
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	workingDir := filepath.Join(homeDir, ".commandforge")

	return &Config{
		LLMProvider:   "openai",
		APIKeys:       make(map[string]string),
		LogLevel:      "info",
		WorkingDir:    workingDir,
		MaxMemorySize: 100,
		Timeout:       60,
	}
}

// LoadConfig loads configuration from a file
func LoadConfig(path string) (*Config, error) {
	config := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// If config file doesn't exist, create one with default values
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return nil, fmt.Errorf("failed to create config directory: %w", err)
			}
			data, err := json.MarshalIndent(config, "", "  ")
			if err != nil {
				return nil, fmt.Errorf("failed to marshal default config: %w", err)
			}
			if err := os.WriteFile(path, data, 0644); err != nil {
				return nil, fmt.Errorf("failed to write default config: %w", err)
			}
			return config, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return config, nil
}

// SaveConfig saves the configuration to a file
func SaveConfig(config *Config, path string) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}
