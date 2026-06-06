package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	)

func TestBuildMCPToolName(t *testing.T) {
	tests := []struct {
		name         string
		serverName   string
		toolName     string
		expected     string
	}{
		{
			name:       "simple server and tool",
			serverName: "github",
			toolName:   "list_issues",
			expected:   "mcp__github__list_issues",
		},
		{
			name:       "server with dots",
			serverName: "my.server",
			toolName:   "get_info",
			expected:   "mcp__my_server__get_info",
		},
		{
			name:       "server with mixed case",
			serverName: "GitHub",
			toolName:   "ListPRs",
			expected:   "mcp__github__ListPRs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildMCPToolName(tt.serverName, tt.toolName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseMCPToolName(t *testing.T) {
	tests := []struct {
		name           string
		qualifiedName  string
		wantServer     string
		wantTool       string
		wantOk         bool
	}{
		{
			name:          "simple qualified name",
			qualifiedName: "mcp__github__list_issues",
			wantServer:    "github",
			wantTool:      "list_issues",
			wantOk:        true,
		},
		{
			name:          "tool with underscores",
			qualifiedName: "mcp__github__tool__with__underscores",
			wantServer:    "github",
			wantTool:      "tool__with__underscores",
			wantOk:        true,
		},
		{
			name:          "normalized server name",
			qualifiedName: "mcp__my_server__echo",
			wantServer:    "my_server",
			wantTool:      "echo",
			wantOk:        true,
		},
		{
			name:          "missing mcp prefix",
			qualifiedName: "github__list_issues",
			wantOk:        false,
		},
		{
			name:          "empty server name",
			qualifiedName: "mcp____tool",
			wantOk:        false,
		},
		{
			name:          "missing double underscore separator",
			qualifiedName: "mcp__github",
			wantOk:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, tool, ok := parseMCPToolName(tt.qualifiedName)
			assert.Equal(t, tt.wantOk, ok)
			if tt.wantOk {
				assert.Equal(t, tt.wantServer, server)
				assert.Equal(t, tt.wantTool, tool)
			}
		})
	}
}

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "dots replaced with underscores",
			input:    "My.Server",
			expected: "my_server",
		},
		{
			name:     "valid name unchanged",
			input:    "valid-name",
			expected: "valid-name",
		},
		{
			name:     "spaces stripped",
			input:    "Has Space",
			expected: "hasspace",
		},
		{
			name:     "mixed case lowered",
			input:    "GitHub-API",
			expected: "github-api",
		},
		{
			name:     "special chars stripped",
			input:    "hello@world!",
			expected: "helloworld",
		},
		{
			name:     "already normalized",
			input:    "filesystem",
			expected: "filesystem",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
		result := normalizeName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMCPToolWrapperImplementsTool(t *testing.T) {
	// MCPToolWrapper has a compile-time check in types.go: var _ tool.Tool = (*MCPToolWrapper)(nil)
	// This test confirms the wrapper can be constructed and returns expected values.
	w := NewMCPToolWrapper(MCPToolInfo{
		Name:        "test_tool",
		Description: "a test tool",
		InputSchema: map[string]any{"type": "object"},
		ServerName:  "test_server",
	}, nil)
	assert.Equal(t, "mcp__test_server__test_tool", w.Name())
	assert.Equal(t, "[test_server] a test tool", w.Description())
	assert.Equal(t, map[string]any{"type": "object"}, w.InputSchema())
}