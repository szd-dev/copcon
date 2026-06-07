package mcp

import (
	"context"
	"log/slog"
	"testing"

	gmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/copcon/core/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPlugin_Name(t *testing.T) {
	p := NewPlugin(nil)
	assert.Equal(t, "mcp", p.Name())
}

func TestNewPlugin_ToolsBeforeInit(t *testing.T) {
	p := NewPlugin(nil)
	assert.Empty(t, p.Tools())
}

func TestNewPlugin_HooksReturnsNil(t *testing.T) {
	p := NewPlugin(nil)
	assert.Nil(t, p.Hooks())
}

func TestNewPlugin_InitInjectsLogger(t *testing.T) {
	logger := slog.Default()
	p := NewPlugin(nil)

	err := p.Init(plugin.PluginDeps{Logger: logger})
	require.NoError(t, err)

	impl := p.(*MCPPlugin)
	assert.Equal(t, logger, impl.logger)
}

func TestNewPlugin_InitDefaultLogger(t *testing.T) {
	p := NewPlugin(nil)

	err := p.Init(plugin.PluginDeps{Logger: nil})
	require.NoError(t, err)

	impl := p.(*MCPPlugin)
	assert.NotNil(t, impl.logger)
}

func TestNewPlugin_ToolsAfterInit(t *testing.T) {
	mgr := NewConnectionManager()
	transport := setupMockServerForPlugin(t, "myserver",
		mockToolDef{name: "echo", desc: "Echo tool"},
		mockToolDef{name: "add", desc: "Add tool"},
	)
	connectMockToManager(t, mgr, "myserver", transport)

	p := NewPluginWithManager([]MCPServerConfig{{Name: "myserver"}}, mgr)

	err := p.Init(plugin.PluginDeps{Logger: slog.Default()})
	require.NoError(t, err)

	tools := p.Tools()
	assert.Len(t, tools, 2)

	toolNames := make(map[string]bool)
	for _, tl := range tools {
		toolNames[tl.Name()] = true
	}
	assert.True(t, toolNames["mcp.tool.myserver__echo"])
	assert.True(t, toolNames["mcp.tool.myserver__add"])
}

func TestNewPlugin_ToolNamingConvention(t *testing.T) {
	mgr := NewConnectionManager()
	transport := setupMockServerForPlugin(t, "github",
		mockToolDef{name: "list_repos", desc: "List repos"},
	)
	connectMockToManager(t, mgr, "github", transport)

	p := NewPluginWithManager([]MCPServerConfig{{Name: "github"}}, mgr)
	err := p.Init(plugin.PluginDeps{Logger: slog.Default()})
	require.NoError(t, err)

	tools := p.Tools()
	require.Len(t, tools, 1)
	assert.Equal(t, "mcp.tool.github__list_repos", tools[0].Name())
}

func TestNewPlugin_AllowedToolsFiltering(t *testing.T) {
	mgr := NewConnectionManager()
	transport := setupMockServerForPlugin(t, "filtered",
		mockToolDef{name: "echo", desc: "Echo tool"},
		mockToolDef{name: "delete", desc: "Delete tool"},
	)
	connectMockToManager(t, mgr, "filtered", transport)

	p := NewPluginWithManager([]MCPServerConfig{{
		Name: "filtered",
		AllowedTools: &AllowedToolsConfig{
			Include: []string{"echo"},
		},
	}}, mgr)

	err := p.Init(plugin.PluginDeps{Logger: slog.Default()})
	require.NoError(t, err)

	tools := p.Tools()
	assert.Len(t, tools, 1)
	assert.Equal(t, "mcp.tool.filtered__echo", tools[0].Name())
}

func TestNewPlugin_PartialFailure(t *testing.T) {
	mgr := NewConnectionManager()
	transport := setupMockServerForPlugin(t, "good-server",
		mockToolDef{name: "echo", desc: "Echo tool"},
	)
	connectMockToManager(t, mgr, "good-server", transport)

	p := NewPluginWithManager([]MCPServerConfig{
		{Name: "good-server"},
		{Name: "missing-server"},
	}, mgr)

	err := p.Init(plugin.PluginDeps{Logger: slog.Default()})
	require.NoError(t, err)

	tools := p.Tools()
	assert.Len(t, tools, 1)
	assert.Equal(t, "mcp.tool.good-server__echo", tools[0].Name())
}

func TestNewPlugin_ConnectionManager(t *testing.T) {
	mgr := NewConnectionManager()
	p := NewPluginWithManager(nil, mgr)
	assert.Equal(t, mgr, p.(*MCPPlugin).ConnectionManager())
}

func TestPluginToolName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "mcp__github__list_repos",
			expected: "mcp.tool.github__list_repos",
		},
		{
			input:    "mcp__my_server__echo",
			expected: "mcp.tool.my_server__echo",
		},
		{
			input:    "mcp__slack__send_message",
			expected: "mcp.tool.slack__send_message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, pluginToolName(tt.input))
		})
	}
}

func TestMcpToolRenamingWrapper(t *testing.T) {
	inner := NewMCPToolWrapper(MCPToolInfo{
		Name:        "test_tool",
		Description: "a test tool",
		InputSchema: map[string]any{"type": "object"},
		ServerName:  "test_server",
	}, nil)

	wrapper := &mcpToolRenamingWrapper{
		Tool:    inner,
		newName: pluginToolName(inner.Name()),
	}

	assert.Equal(t, "mcp.tool.test_server__test_tool", wrapper.Name())
	assert.Equal(t, "[test_server] a test tool", wrapper.Description())
	assert.Equal(t, map[string]any{"type": "object"}, wrapper.InputSchema())
}

func TestNewPlugin_IntegrationExecution(t *testing.T) {
	ctx := context.Background()

	server := gmcp.NewServer(&gmcp.Implementation{Name: "exec-server", Version: "1.0.0"}, nil)
	type echoArgs struct {
		Message string `json:"message"`
	}
	gmcp.AddTool(server, &gmcp.Tool{
		Name:        "echo",
		Description: "Echo back",
	}, func(_ context.Context, _ *gmcp.CallToolRequest, args echoArgs) (*gmcp.CallToolResult, any, error) {
		return &gmcp.CallToolResult{
			Content: []gmcp.Content{&gmcp.TextContent{Text: args.Message}},
		}, nil, nil
	})

	serverTransport, clientTransport := gmcp.NewInMemoryTransports()
	_, err := server.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)

	mgr := NewConnectionManager()
	_, err = mgr.ConnectWithTransport(ctx, "exec-server", clientTransport)
	require.NoError(t, err)

	p := NewPluginWithManager([]MCPServerConfig{{Name: "exec-server"}}, mgr)
	err = p.Init(plugin.PluginDeps{Logger: slog.Default()})
	require.NoError(t, err)

	tools := p.Tools()
	require.Len(t, tools, 1)
	assert.Equal(t, "mcp.tool.exec-server__echo", tools[0].Name())

	chatCtx := &mockChatContext{Ctx: ctx}
	result, err := tools[0].Execute(chatCtx, map[string]any{"message": "hello"})
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, "hello", result.Data)
}
