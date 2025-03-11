package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileTool provides functionality to interact with the filesystem
type FileTool struct {
	*BaseTool
	BaseDir string
}

// FileResult represents the result of a file operation
type FileResult struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// NewFileTool creates a new file tool
func NewFileTool(baseDir string) *FileTool {
	return &FileTool{
		BaseTool: NewBaseTool(
			"file",
			"Interact with the filesystem (read, write, list files)",
		),
		BaseDir: baseDir,
	}
}

// Execute performs file operations
func (t *FileTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// Get the operation from parameters
	operation, ok := params["operation"].(string)
	if !ok || operation == "" {
		return nil, fmt.Errorf("operation parameter is required and must be a string")
	}
	
	// Perform the requested operation
	switch operation {
	case "read":
		return t.readFile(params)
	case "write":
		return t.writeFile(params)
	case "list":
		return t.listFiles(params)
	case "delete":
		return t.deleteFile(params)
	default:
		return nil, fmt.Errorf("unknown operation: %s", operation)
	}
}

// readFile reads the content of a file
func (t *FileTool) readFile(params map[string]interface{}) (interface{}, error) {
	// Get the file path
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("path parameter is required and must be a string")
	}
	
	// Ensure the path is within the base directory
	fullPath := filepath.Join(t.BaseDir, path)
	if !isPathSafe(fullPath, t.BaseDir) {
		return nil, fmt.Errorf("path is outside the allowed directory")
	}
	
	// Read the file
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return &FileResult{
			Success: false,
			Message: fmt.Sprintf("failed to read file: %v", err),
		}, nil
	}
	
	return &FileResult{
		Success: true,
		Message: "file read successfully",
		Data:    string(data),
	}, nil
}

// writeFile writes content to a file
func (t *FileTool) writeFile(params map[string]interface{}) (interface{}, error) {
	// Get the file path
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("path parameter is required and must be a string")
	}
	
	// Get the content
	content, ok := params["content"].(string)
	if !ok {
		return nil, fmt.Errorf("content parameter is required and must be a string")
	}
	
	// Ensure the path is within the base directory
	fullPath := filepath.Join(t.BaseDir, path)
	if !isPathSafe(fullPath, t.BaseDir) {
		return nil, fmt.Errorf("path is outside the allowed directory")
	}
	
	// Create the directory if it doesn't exist
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return &FileResult{
			Success: false,
			Message: fmt.Sprintf("failed to create directory: %v", err),
		}, nil
	}
	
	// Write the file
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return &FileResult{
			Success: false,
			Message: fmt.Sprintf("failed to write file: %v", err),
		}, nil
	}
	
	return &FileResult{
		Success: true,
		Message: "file written successfully",
		Data:    fullPath,
	}, nil
}

// listFiles lists files in a directory
func (t *FileTool) listFiles(params map[string]interface{}) (interface{}, error) {
	// Get the directory path
	path, ok := params["path"].(string)
	if !ok {
		path = "" // Default to base directory
	}
	
	// Ensure the path is within the base directory
	fullPath := filepath.Join(t.BaseDir, path)
	if !isPathSafe(fullPath, t.BaseDir) {
		return nil, fmt.Errorf("path is outside the allowed directory")
	}
	
	// Read the directory
	files, err := os.ReadDir(fullPath)
	if err != nil {
		return &FileResult{
			Success: false,
			Message: fmt.Sprintf("failed to list directory: %v", err),
		}, nil
	}
	
	// Create a list of file info
	fileList := make([]map[string]interface{}, 0, len(files))
	for _, file := range files {
		// Get detailed file info
		info, err := file.Info()
		if err != nil {
			continue
		}
		
		fileInfo := map[string]interface{}{
			"name":      file.Name(),
			"size":      info.Size(),
			"is_dir":    file.IsDir(),
			"modified":  info.ModTime().Format("2006-01-02 15:04:05"),
			"full_path": filepath.Join(fullPath, file.Name()),
		}
		fileList = append(fileList, fileInfo)
	}
	
	return &FileResult{
		Success: true,
		Message: fmt.Sprintf("listed %d files", len(files)),
		Data:    fileList,
	}, nil
}

// deleteFile deletes a file or directory
func (t *FileTool) deleteFile(params map[string]interface{}) (interface{}, error) {
	// Get the file path
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("path parameter is required and must be a string")
	}
	
	// Ensure the path is within the base directory
	fullPath := filepath.Join(t.BaseDir, path)
	if !isPathSafe(fullPath, t.BaseDir) {
		return nil, fmt.Errorf("path is outside the allowed directory")
	}
	
	// Delete the file or directory
	if err := os.RemoveAll(fullPath); err != nil {
		return &FileResult{
			Success: false,
			Message: fmt.Sprintf("failed to delete: %v", err),
		}, nil
	}
	
	return &FileResult{
		Success: true,
		Message: "deleted successfully",
	}, nil
}

// isPathSafe checks if a path is within the allowed base directory
func isPathSafe(path, baseDir string) bool {
	// Get absolute paths
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	
	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return false
	}
	
	// Check if the path is within the base directory
	relPath, err := filepath.Rel(absBaseDir, absPath)
	if err != nil {
		return false
	}
	
	return !filepath.IsAbs(relPath) && !strings.HasPrefix(relPath, "..")
}
