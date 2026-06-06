package mcp

import (
	"context"
	"fmt"
	"strings"
	"testing"

	gmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/copcon/core/entity"
	"github.com/copcon/core/iface"
	"github.com/copcon/core/plugin"
	"github.com/copcon/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockChatContext struct {
	ctx context.Context
}

func (m *mockChatContext) Context() context.Context                          { return m.ctx }
func (m *mockChatContext) SessionID() string                                 { return "" }
func (m *mockChatContext) AgentID() string                                   { return "" }
func (m *mockChatContext) Events() <-chan entity.Event                       { return nil }
func (m *mockChatContext) Emit(event entity.Event)                           {}
func (m *mockChatContext) Close()                                            {}
func (m *mockChatContext) Closed() <-chan struct{}                           { return nil }
func (m *mockChatContext) Depth() int                                        { return 0 }
func (m *mockChatContext) Subscribe(fromSeq int64) (*iface.Subscriber, bool) { return nil, false }
func (m *mockChatContext) RequestInput(req iface.InputRequest) (*iface.InputResponse, error) {
	return nil, nil
}
func (m *mockChatContext) ResolveInput(id string, resp *iface.InputResponse) error {
	return nil
}
func (m *mockChatContext) PendingInputs() []iface.InputRequest { return nil }
func (m *mockChatContext) SetPartLocator(string, int, int)     {}
func (m *mockChatContext) ClearPartLocator()                   {}

func setupIntegrationServer(t *testing.T, name string) gmcp.Transport {
	t.Helper()
	ctx := context.Background()

	server := gmcp.NewServer(&gmcp.Implementation{Name: name, Version: "1.0.0"}, nil)

	type greetArgs struct {
		Name string `json:"name"`
	}
	gmcp.AddTool(server, &gmcp.Tool{
		Name:        "greet",
		Description: "Greet a person by name",
	}, func(_ context.Context, _ *gmcp.CallToolRequest, args greetArgs) (*gmcp.CallToolResult, any, error) {
		return &gmcp.CallToolResult{
			Content: []gmcp.Content{&gmcp.TextContent{Text: "Hello, " + args.Name + "!"}},
		}, nil, nil
	})

	type addArgs struct {
		A float64 `json:"a"`
		B float64 `json:"b"`
	}
	gmcp.AddTool(server, &gmcp.Tool{
		Name:        "add",
		Description: "Add two numbers",
	}, func(_ context.Context, _ *gmcp.CallToolRequest, args addArgs) (*gmcp.CallToolResult, any, error) {
		result := args.A + args.B
		var text string
		if args.A == float64(int64(args.A)) && args.B == float64(int64(args.B)) {
			text = fmt.Sprintf("%d", int64(result))
		} else {
			text = fmt.Sprintf("%g", result)
		}
		return &gmcp.CallToolResult{
			Content: []gmcp.Content{&gmcp.TextContent{Text: text}},
		}, nil, nil
	})

	type uppercaseArgs struct {
		Text string `json:"text"`
	}
	gmcp.AddTool(server, &gmcp.Tool{
		Name:        "uppercase",
		Description: "Convert text to uppercase",
	}, func(_ context.Context, _ *gmcp.CallToolRequest, args uppercaseArgs) (*gmcp.CallToolResult, any, error) {
		return &gmcp.CallToolResult{
			Content: []gmcp.Content{&gmcp.TextContent{Text: strings.ToUpper(args.Text)}},
		}, nil, nil
	})

	serverTransport, clientTransport := gmcp.NewInMemoryTransports()
	_, err := server.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)

	return clientTransport
}

func TestIntegration_EndToEnd(t *testing.T) {
	ctx := context.Background()

	transport := setupIntegrationServer(t, "integration-server")

	mgr := NewConnectionManager()
	_, err := mgr.ConnectWithTransport(ctx, "integration-server", transport)
	require.NoError(t, err)

	p := NewPluginWithManager([]MCPServerConfig{{Name: "integration-server"}}, mgr)
	require.NoError(t, p.Init(plugin.PluginDeps{}))

	tools := p.Tools()
	assert.Len(t, tools, 3)

	toolMap := make(map[string]tool.Tool)
	for _, tl := range tools {
		toolMap[tl.Name()] = tl
	}

	greetTool, ok := toolMap["mcp.tool.integration-server__greet"]
	require.True(t, ok, "greet tool should exist")
	addTool, ok := toolMap["mcp.tool.integration-server__add"]
	require.True(t, ok, "add tool should exist")
	upperTool, ok := toolMap["mcp.tool.integration-server__uppercase"]
	require.True(t, ok, "uppercase tool should exist")

	chatCtx := &mockChatContext{ctx: ctx}

	greetResult, err := greetTool.Execute(chatCtx, map[string]any{"name": "World"})
	require.NoError(t, err)
	assert.True(t, greetResult.Success)
	assert.Equal(t, "Hello, World!", greetResult.Data)

	addResult, err := addTool.Execute(chatCtx, map[string]any{"a": 3.0, "b": 4.0})
	require.NoError(t, err)
	assert.True(t, addResult.Success)
	assert.Equal(t, "7", addResult.Data)

	upperResult, err := upperTool.Execute(chatCtx, map[string]any{"text": "hello"})
	require.NoError(t, err)
	assert.True(t, upperResult.Success)
	assert.Equal(t, "HELLO", upperResult.Data)
}

func TestIntegration_MCPWithBuiltins(t *testing.T) {
	mgr := NewConnectionManager()
	transport := setupMockServerForPlugin(t, "test",
		mockToolDef{name: "echo", desc: "Echo tool"},
		mockToolDef{name: "add", desc: "Add tool"},
	)
	connectMockToManager(t, mgr, "test", transport)

	p := NewPluginWithManager([]MCPServerConfig{{Name: "test"}}, mgr)
	require.NoError(t, p.Init(plugin.PluginDeps{}))

	tools := p.Tools()
	for _, tc := range tools {
		assert.True(t, strings.HasPrefix(tc.Name(), "mcp.tool."), "all MCP tools should have mcp.tool. prefix, got: %s", tc.Name())
	}
}

func setupMockServerForPlugin(t *testing.T, name string, tools ...mockToolDef) gmcp.Transport {
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
