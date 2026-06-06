package mcp

import (
	"context"
	"testing"

	gmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/copcon/core/capabilities"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupMockServerForModule(t *testing.T, name string, tools ...mockToolDef) gmcp.Transport {
	t.Helper()
	ctx := context.Background()

	server := gmcp.NewServer(&gmcp.Implementation{Name: name, Version: "1.0.0"}, nil)

	for _, td := range tools {
		td := td
		gmcp.AddTool(server, &gmcp.Tool{
			Name:        td.name,
			Description: td.desc,
		}, func(_ context.Context, _ *gmcp.CallToolRequest, args map[string]any) (*gmcp.CallToolResult, any, error) {
			msg, _ := args["message"].(string)
			if msg == "" {
				msg = "ok"
			}
			return &gmcp.CallToolResult{
				Content: []gmcp.Content{&gmcp.TextContent{Text: msg}},
			}, nil, nil
		})
	}

	serverTransport, clientTransport := gmcp.NewInMemoryTransports()
	_, err := server.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)

	return clientTransport
}

type mockToolDef struct {
	name string
	desc string
}

func connectMockToManager(t *testing.T, mgr *ConnectionManager, name string, transport gmcp.Transport) {
	t.Helper()
	ctx := context.Background()
	_, err := mgr.ConnectWithTransport(ctx, name, transport)
	require.NoError(t, err)
}

func TestNewTools(t *testing.T) {
	mgr := NewConnectionManager()
	transport := setupMockServerForModule(t, "myserver",
		mockToolDef{name: "echo", desc: "Echo tool"},
		mockToolDef{name: "add", desc: "Add tool"},
	)
	connectMockToManager(t, mgr, "myserver", transport)

	module := NewMCPModuleWithManager([]MCPServerConfig{
		{Name: "myserver"},
	}, mgr)

	tools, err := module.NewTools(capabilities.CapabilityDeps{})
	require.NoError(t, err)

	toolNames := make(map[string]bool)
	for _, tl := range tools {
		toolNames[tl.Name()] = true
	}
	assert.True(t, toolNames["mcp__myserver__echo"], "expected echo tool")
	assert.True(t, toolNames["mcp__myserver__add"], "expected add tool")
}

func TestAllowedToolsInclude(t *testing.T) {
	mgr := NewConnectionManager()
	transport := setupMockServerForModule(t, "filtered",
		mockToolDef{name: "echo", desc: "Echo tool"},
		mockToolDef{name: "add", desc: "Add tool"},
		mockToolDef{name: "delete", desc: "Delete tool"},
	)
	connectMockToManager(t, mgr, "filtered", transport)

	module := NewMCPModuleWithManager([]MCPServerConfig{
		{
			Name: "filtered",
			AllowedTools: &AllowedToolsConfig{
				Include: []string{"echo", "add"},
			},
		},
	}, mgr)

	tools, err := module.NewTools(capabilities.CapabilityDeps{})
	require.NoError(t, err)

	toolNames := make(map[string]bool)
	for _, tl := range tools {
		toolNames[tl.Name()] = true
	}
	assert.True(t, toolNames["mcp__filtered__echo"], "echo should be included")
	assert.True(t, toolNames["mcp__filtered__add"], "add should be included")
	assert.False(t, toolNames["mcp__filtered__delete"], "delete should be excluded")
}

func TestAllowedToolsExclude(t *testing.T) {
	mgr := NewConnectionManager()
	transport := setupMockServerForModule(t, "excluded",
		mockToolDef{name: "echo", desc: "Echo tool"},
		mockToolDef{name: "delete", desc: "Delete tool"},
	)
	connectMockToManager(t, mgr, "excluded", transport)

	module := NewMCPModuleWithManager([]MCPServerConfig{
		{
			Name: "excluded",
			AllowedTools: &AllowedToolsConfig{
				Exclude: []string{"delete"},
			},
		},
	}, mgr)

	tools, err := module.NewTools(capabilities.CapabilityDeps{})
	require.NoError(t, err)

	toolNames := make(map[string]bool)
	for _, tl := range tools {
		toolNames[tl.Name()] = true
	}
	assert.True(t, toolNames["mcp__excluded__echo"], "echo should be present")
	assert.False(t, toolNames["mcp__excluded__delete"], "delete should be excluded")
}

func TestPartialFailure(t *testing.T) {
	mgr := NewConnectionManager()
	transport := setupMockServerForModule(t, "good-server",
		mockToolDef{name: "echo", desc: "Echo tool"},
	)
	connectMockToManager(t, mgr, "good-server", transport)

	module := NewMCPModuleWithManager([]MCPServerConfig{
		{Name: "good-server"},
		{Name: "bad-server", Type: TransportStdio, Command: "/nonexistent/command"},
	}, mgr)

	tools, err := module.NewTools(capabilities.CapabilityDeps{})
	require.NoError(t, err)

	assert.Len(t, tools, 1)
	assert.Equal(t, "mcp__good-server__echo", tools[0].Name())
}

func TestNewHooksReturnsNil(t *testing.T) {
	module := NewMCPModule(nil)
	hooks, err := module.NewHooks(capabilities.CapabilityDeps{})
	assert.NoError(t, err)
	assert.Nil(t, hooks)
}

func TestIsToolAllowed(t *testing.T) {
	tests := []struct {
		name   string
		config *AllowedToolsConfig
		tool   string
		want   bool
	}{
		{
			name: "nil config allows all",
			config: nil,
			tool:   "anything",
			want:   true,
		},
		{
			name:   "empty config allows all",
			config: &AllowedToolsConfig{},
			tool:   "anything",
			want:   true,
		},
		{
			name:   "include matches",
			config: &AllowedToolsConfig{Include: []string{"echo", "add"}},
			tool:   "echo",
			want:   true,
		},
		{
			name:   "include no match",
			config: &AllowedToolsConfig{Include: []string{"echo"}},
			tool:   "delete",
			want:   false,
		},
		{
			name:   "exclude matches",
			config: &AllowedToolsConfig{Exclude: []string{"delete"}},
			tool:   "delete",
			want:   false,
		},
		{
			name:   "exclude no match",
			config: &AllowedToolsConfig{Exclude: []string{"delete"}},
			tool:   "echo",
			want:   true,
		},
		{
			name:   "include and exclude — exclude wins",
			config: &AllowedToolsConfig{Include: []string{"echo", "delete"}, Exclude: []string{"delete"}},
			tool:   "delete",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isToolAllowed(tt.tool, tt.config)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestModuleCapabilityInterface(t *testing.T) {
	module := NewMCPModule(nil)
	assert.Equal(t, "modules.mcp", module.Name())
	assert.Equal(t, capabilities.CapabilityTypeModule, module.Type())
	assert.Nil(t, module.DependsOn())
}
