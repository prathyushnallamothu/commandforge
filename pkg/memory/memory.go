package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MemoryItem represents a single memory item
type MemoryItem struct {
	Key       string      `json:"key"`
	Value     interface{} `json:"value"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// FileMemory implements memory storage using the filesystem
type FileMemory struct {
	BasePath string
	mutex    sync.RWMutex
	cache    map[string]*MemoryItem
}

// NewFileMemory creates a new file-based memory store
func NewFileMemory(basePath string) (*FileMemory, error) {
	// Ensure the directory exists
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create memory directory: %w", err)
	}
	
	memory := &FileMemory{
		BasePath: basePath,
		cache:    make(map[string]*MemoryItem),
	}
	
	// Load existing memory items
	if err := memory.loadFromDisk(); err != nil {
		return nil, err
	}
	
	return memory, nil
}

// loadFromDisk loads all memory items from disk
func (m *FileMemory) loadFromDisk() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	// Clear the cache
	m.cache = make(map[string]*MemoryItem)
	
	// Walk the directory and load all files
	return filepath.Walk(m.BasePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// Skip directories
		if info.IsDir() {
			return nil
		}
		
		// Skip non-json files
		if filepath.Ext(path) != ".json" {
			return nil
		}
		
		// Read the file
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read memory file %s: %w", path, err)
		}
		
		// Parse the memory item
		var item MemoryItem
		if err := json.Unmarshal(data, &item); err != nil {
			return fmt.Errorf("failed to parse memory file %s: %w", path, err)
		}
		
		// Add to cache
		m.cache[item.Key] = &item
		
		return nil
	})
}

// getFilePath returns the file path for a memory key
func (m *FileMemory) getFilePath(key string) string {
	// Sanitize the key to be a valid filename
	sanitized := filepath.Base(key)
	return filepath.Join(m.BasePath, sanitized+".json")
}

// Save stores a memory item
func (m *FileMemory) Save(ctx context.Context, key string, value interface{}) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	// Check if the item already exists
	item, exists := m.cache[key]
	if !exists {
		// Create a new item
		item = &MemoryItem{
			Key:       key,
			CreatedAt: time.Now(),
		}
	}
	
	// Update the item
	item.Value = value
	item.UpdatedAt = time.Now()
	
	// Save to cache
	m.cache[key] = item
	
	// Save to disk
	data, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal memory item: %w", err)
	}
	
	filePath := m.getFilePath(key)
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write memory file: %w", err)
	}
	
	return nil
}

// Load retrieves a memory item
func (m *FileMemory) Load(ctx context.Context, key string) (interface{}, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	// Check if the item exists in cache
	item, exists := m.cache[key]
	if !exists {
		return nil, fmt.Errorf("memory item not found: %s", key)
	}
	
	return item.Value, nil
}

// List returns all memory keys
func (m *FileMemory) List(ctx context.Context) ([]string, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	keys := make([]string, 0, len(m.cache))
	for key := range m.cache {
		keys = append(keys, key)
	}
	
	return keys, nil
}

// Delete removes a memory item
func (m *FileMemory) Delete(ctx context.Context, key string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	// Check if the item exists
	_, exists := m.cache[key]
	if !exists {
		return fmt.Errorf("memory item not found: %s", key)
	}
	
	// Remove from cache
	delete(m.cache, key)
	
	// Remove from disk
	filePath := m.getFilePath(key)
	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("failed to delete memory file: %w", err)
	}
	
	return nil
}

// InMemory implements an in-memory storage
type InMemory struct {
	mutex sync.RWMutex
	items map[string]*MemoryItem
}

// NewInMemory creates a new in-memory store
func NewInMemory() *InMemory {
	return &InMemory{
		items: make(map[string]*MemoryItem),
	}
}

// Save stores a memory item
func (m *InMemory) Save(ctx context.Context, key string, value interface{}) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	// Check if the item already exists
	item, exists := m.items[key]
	if !exists {
		// Create a new item
		item = &MemoryItem{
			Key:       key,
			CreatedAt: time.Now(),
		}
	}
	
	// Update the item
	item.Value = value
	item.UpdatedAt = time.Now()
	
	// Save to memory
	m.items[key] = item
	
	return nil
}

// Load retrieves a memory item
func (m *InMemory) Load(ctx context.Context, key string) (interface{}, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	// Check if the item exists
	item, exists := m.items[key]
	if !exists {
		return nil, fmt.Errorf("memory item not found: %s", key)
	}
	
	return item.Value, nil
}

// List returns all memory keys
func (m *InMemory) List(ctx context.Context) ([]string, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	keys := make([]string, 0, len(m.items))
	for key := range m.items {
		keys = append(keys, key)
	}
	
	return keys, nil
}

// Delete removes a memory item
func (m *InMemory) Delete(ctx context.Context, key string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	// Check if the item exists
	_, exists := m.items[key]
	if !exists {
		return fmt.Errorf("memory item not found: %s", key)
	}
	
	// Remove from memory
	delete(m.items, key)
	
	return nil
}
