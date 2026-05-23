package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/iface"
	"github.com/copcon/core/tool"
)

const (
	MaxFileSize = 10 * 1024 * 1024
)

var forbiddenPaths = []string{
	"/etc",
	"/root",
	"/home",
	"/var",
	"/usr",
	"/bin",
	"/sbin",
	"/lib",
}

type FileOps struct {
	workDir string
}

func NewFileOps(workDir string) *FileOps {
	if workDir == "" {
		workDir, _ = os.Getwd()
	}
	return &FileOps{workDir: workDir}
}

func (t *FileOps) Name() string {
	return "file_ops"
}

func (t *FileOps) Description() string {
	return "Read and write files within the working directory"
}

func (t *FileOps) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{
				"type":        "string",
				"enum":        []string{"read", "write", "list"},
				"description": "File operation to perform",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "File or directory path",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Content to write (for write operation)",
			},
		},
		"required": []string{"operation", "path"},
	}
}

func (t *FileOps) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
	operation, ok := args["operation"].(string)
	if !ok {
		return &tool.ToolResult{Success: false, Error: "operation is required"}, nil
	}

	path, ok := args["path"].(string)
	if !ok {
		return &tool.ToolResult{Success: false, Error: "path is required"}, nil
	}

	switch operation {
	case "read":
		return t.readFile(path)
	case "write":
		content, ok := args["content"].(string)
		if !ok {
			return &tool.ToolResult{Success: false, Error: "content is required for write"}, nil
		}
		return t.writeFile(path, content)
	case "list":
		return t.listDir(path)
	default:
		return &tool.ToolResult{Success: false, Error: fmt.Sprintf("unknown operation: %s", operation)}, nil
	}
}

func (t *FileOps) isPathAllowed(path string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	for _, forbidden := range forbiddenPaths {
		if strings.HasPrefix(absPath, forbidden) {
			return false
		}
	}

	workDirAbs, _ := filepath.Abs(t.workDir)
	return strings.HasPrefix(absPath, workDirAbs)
}

func (t *FileOps) readFile(path string) (*tool.ToolResult, error) {
	info, err := os.Stat(path)
	if err != nil {
		return &tool.ToolResult{Success: false, Error: err.Error()}, nil
	}

	if info.Size() > MaxFileSize {
		return &tool.ToolResult{Success: false, Error: "file too large (max 10MB)"}, nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return &tool.ToolResult{Success: false, Error: err.Error()}, nil
	}

	return &tool.ToolResult{
		Success: true,
		Data: map[string]any{
			"content": string(content),
			"size":    info.Size(),
		},
	}, nil
}

func (t *FileOps) writeFile(path, content string) (*tool.ToolResult, error) {
	if len(content) > MaxFileSize {
		return &tool.ToolResult{Success: false, Error: "content too large (max 10MB)"}, nil
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return &tool.ToolResult{Success: false, Error: err.Error()}, nil
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return &tool.ToolResult{Success: false, Error: err.Error()}, nil
	}

	return &tool.ToolResult{
		Success: true,
		Data: map[string]any{
			"path": path,
			"size": len(content),
		},
	}, nil
}

func (t *FileOps) listDir(path string) (*tool.ToolResult, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return &tool.ToolResult{Success: false, Error: err.Error()}, nil
	}

	files := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, map[string]any{
			"name":  entry.Name(),
			"isDir": entry.IsDir(),
			"size":  info.Size(),
		})
	}

	return &tool.ToolResult{
		Success: true,
		Data: map[string]any{
			"files": files,
			"count": len(files),
		},
	}, nil
}

func init() {
	capabilities.Register(&fileOpsCapability{})
}

type fileOpsCapability struct{}

func (c *fileOpsCapability) Name() string                         { return "tools.file_ops" }
func (c *fileOpsCapability) Type() capabilities.CapabilityType    { return capabilities.CapabilityTypeTool }
func (c *fileOpsCapability) DependsOn() []string                  { return nil }
func (c *fileOpsCapability) NewTool(deps capabilities.CapabilityDeps) (tool.Tool, error) {
	return NewFileOps(""), nil
}