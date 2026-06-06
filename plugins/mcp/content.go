package mcp

import (
	"context"
	"fmt"
	"strings"

	gmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// extractMCPContent extracts text from an MCP CallToolResult.
// It handles TextContent and falls back to a string representation
// for other content types (images, audio, etc.).
func extractMCPContent(result *gmcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	var parts []string
	for _, c := range result.Content {
		switch v := c.(type) {
		case *gmcp.TextContent:
			parts = append(parts, v.Text)
		default:
			parts = append(parts, fmt.Sprintf("[unsupported content type: %T]", c))
		}
	}
	return strings.Join(parts, "\n")
}

// buildMCPToolCallFunc creates an MCPToolCallFunc that uses the given
// ClientSession to invoke tools on the MCP server.
func buildMCPToolCallFunc(session *gmcp.ClientSession) MCPToolCallFunc {
	return func(ctx context.Context, toolName string, args map[string]any) (any, error) {
		result, err := session.CallTool(ctx, &gmcp.CallToolParams{
			Name:      toolName,
			Arguments: args,
		})
		if err != nil {
			return nil, err
		}
		return extractMCPContent(result), nil
	}
}
