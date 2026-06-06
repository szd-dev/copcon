package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/copcon/core/iface"
	"github.com/copcon/core/tool"
)

// MCPToolInfo holds metadata about a discovered MCP tool.
type MCPToolInfo struct {
	// Name is the original tool name from the MCP server.
	Name string

	// Description is a human-readable description of what the tool does.
	Description string

	// InputSchema is the JSON Schema (as a map) describing the tool's
	// expected arguments.
	InputSchema map[string]any

	// ServerName identifies the MCP server that provides this tool.
	ServerName string
}

// MCPToolCallFunc is the function signature for invoking an MCP tool.
type MCPToolCallFunc func(ctx context.Context, toolName string, args map[string]any) (any, error)

// MCPToolWrapper implements tool.Tool, wrapping an MCP tool so it can be
// registered with the CopCon tool manager and invoked by the agent.
type MCPToolWrapper struct {
	info     MCPToolInfo
	callFunc MCPToolCallFunc
}

// NewMCPToolWrapper creates a new wrapper for an MCP tool.
func NewMCPToolWrapper(info MCPToolInfo, callFunc MCPToolCallFunc) *MCPToolWrapper {
	return &MCPToolWrapper{info: info, callFunc: callFunc}
}

// Name returns the qualified tool name in the format
// "mcp__{serverName}__{toolName}".
func (w *MCPToolWrapper) Name() string {
	return buildMCPToolName(w.info.ServerName, w.info.Name)
}

// Description returns the tool description prefixed with the server name
// in the format "[{serverName}] {description}".
func (w *MCPToolWrapper) Description() string {
	return fmt.Sprintf("[%s] %s", w.info.ServerName, w.info.Description)
}

// InputSchema returns the tool's JSON input schema.
func (w *MCPToolWrapper) InputSchema() map[string]any {
	return w.info.InputSchema
}

// Execute invokes the MCP tool via the callFunc and wraps the result.
func (w *MCPToolWrapper) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
	data, err := w.callFunc(chatCtx.Context(), w.info.Name, args)
	if err != nil {
		return &tool.ToolResult{Success: false, Error: err.Error()}, nil
	}
	return &tool.ToolResult{Success: true, Data: data}, nil
}

// buildMCPToolName creates the qualified tool name from a server name and
// tool name. The format is "mcp__{serverName}__{toolName}".
func buildMCPToolName(serverName, toolName string) string {
	return "mcp__" + normalizeName(serverName) + "__" + toolName
}

// parseMCPToolName extracts the server and tool name from a qualified
// tool name of the form "mcp__{serverName}__{toolName}".
func parseMCPToolName(qualifiedName string) (serverName, toolName string, ok bool) {
	if !strings.HasPrefix(qualifiedName, "mcp__") {
		return "", "", false
	}
	rest := strings.TrimPrefix(qualifiedName, "mcp__")
	serverName, toolName, ok = strings.Cut(rest, "__")
	if !ok || serverName == "" || toolName == "" {
		return "", "", false
	}
	return serverName, toolName, true
}

// normalizeName ensures the name is a valid tool name component by
// lowercasing, replacing '.' with '_', and stripping invalid characters.
func normalizeName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, ".", "_")
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Compile-time check that MCPToolWrapper implements tool.Tool.
var _ tool.Tool = (*MCPToolWrapper)(nil)

func isToolAllowed(name string, config *AllowedToolsConfig) bool {
	if config == nil {
		return true
	}
	if len(config.Include) > 0 {
		found := false
		for _, inc := range config.Include {
			if inc == name {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	for _, exc := range config.Exclude {
		if exc == name {
			return false
		}
	}
	return true
}

func convertSchema(schema any) map[string]any {
	switch v := schema.(type) {
	case map[string]any:
		return v
	default:
		return map[string]any{"type": "object"}
	}
}