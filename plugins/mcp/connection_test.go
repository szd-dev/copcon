package mcp

import (
	"context"
	"sync"
	"testing"

	gmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupConnectedSession(t *testing.T, mgr *ConnectionManager, name string) *gmcp.ClientSession {
	t.Helper()
	ctx := context.Background()

	server := gmcp.NewServer(&gmcp.Implementation{Name: name, Version: "1.0.0"}, nil)

	type echoArgs struct {
		Message string `json:"message"`
	}
	gmcp.AddTool(server, &gmcp.Tool{
		Name:        "echo",
		Description: "Echo the message back",
	}, func(_ context.Context, _ *gmcp.CallToolRequest, args echoArgs) (*gmcp.CallToolResult, any, error) {
		return &gmcp.CallToolResult{
			Content: []gmcp.Content{&gmcp.TextContent{Text: args.Message}},
		}, nil, nil
	})

	serverTransport, clientTransport := gmcp.NewInMemoryTransports()
	_, err := server.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)

	session, err := mgr.ConnectWithTransport(ctx, name, clientTransport)
	require.NoError(t, err)
	return session
}

func TestConnectWithTransport(t *testing.T) {
	mgr := NewConnectionManager()
	session := setupConnectedSession(t, mgr, "test-server")
	defer session.Close()

	got, err := mgr.GetSession("test-server")
	require.NoError(t, err)
	assert.Equal(t, session, got)
}

func TestDisconnect(t *testing.T) {
	mgr := NewConnectionManager()
	setupConnectedSession(t, mgr, "test-server")

	err := mgr.Disconnect("test-server")
	require.NoError(t, err)

	_, err = mgr.GetSession("test-server")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "server not connected")
}

func TestDisconnectNonexistent(t *testing.T) {
	mgr := NewConnectionManager()
	err := mgr.Disconnect("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "server not connected")
}

func TestAlreadyConnected(t *testing.T) {
	mgr := NewConnectionManager()
	setupConnectedSession(t, mgr, "test-server")

	ctx := context.Background()
	server := gmcp.NewServer(&gmcp.Implementation{Name: "test-server", Version: "1.0.0"}, nil)
	_, clientTransport := gmcp.NewInMemoryTransports()
	_, err := server.Connect(ctx, clientTransport, nil)

	_, err = mgr.ConnectWithTransport(ctx, "test-server", clientTransport)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "server already connected")
}

func TestListSessions(t *testing.T) {
	mgr := NewConnectionManager()

	assert.Empty(t, mgr.ListSessions())

	setupConnectedSession(t, mgr, "server-a")
	setupConnectedSession(t, mgr, "server-b")

	list := mgr.ListSessions()
	assert.Len(t, list, 2)
	assert.Contains(t, list, "server-a")
	assert.Contains(t, list, "server-b")
}

func TestDisconnectAll(t *testing.T) {
	mgr := NewConnectionManager()
	setupConnectedSession(t, mgr, "server-a")
	setupConnectedSession(t, mgr, "server-b")

	mgr.DisconnectAll()
	assert.Empty(t, mgr.ListSessions())
}

func TestConcurrentAccess(t *testing.T) {
	mgr := NewConnectionManager()
	setupConnectedSession(t, mgr, "concurrent-server")

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := mgr.GetSession("concurrent-server")
			assert.NoError(t, err)
		}()
	}
	wg.Wait()
}

func TestCreateTransportValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     MCPServerConfig
		wantErr string
	}{
		{
			name:    "stdio without command",
			cfg:     MCPServerConfig{Name: "test", Type: TransportStdio},
			wantErr: "stdio transport requires command",
		},
		{
			name:    "SSE without URL",
			cfg:     MCPServerConfig{Name: "test", Type: TransportSSE},
			wantErr: "SSE transport requires URL",
		},
		{
			name:    "streamable-http without URL",
			cfg:     MCPServerConfig{Name: "test", Type: TransportStreamableHTTP},
			wantErr: "streamable-http transport requires URL",
		},
		{
			name:    "unsupported type",
			cfg:     MCPServerConfig{Name: "test", Type: "websocket"},
			wantErr: "unsupported transport type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := createTransport(tt.cfg)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
