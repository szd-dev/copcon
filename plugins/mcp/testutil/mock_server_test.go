package testutil_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupMockServer creates a server with echo and add tools, connects a client,
// and returns the client session and a cleanup function.
func setupMockServer(t *testing.T) (*mcp.ClientSession, func()) {
	ctx := context.Background()

	// Create the server with two tools.
	server := mcp.NewServer(&mcp.Implementation{Name: "mock", Version: "1.0.0"}, nil)

	type echoArgs struct {
		Message string `json:"message"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "echo",
		Description: "Echo the given message back to the caller",
	}, func(_ context.Context, _ *mcp.CallToolRequest, args echoArgs) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: args.Message}},
		}, nil, nil
	})

	type addArgs struct {
		A float64 `json:"a"`
		B float64 `json:"b"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "add",
		Description: "Add two numbers and return the result",
	}, func(_ context.Context, _ *mcp.CallToolRequest, args addArgs) (*mcp.CallToolResult, any, error) {
		result := args.A + args.B
		var text string
		if args.A == float64(int64(args.A)) && args.B == float64(int64(args.B)) {
			text = fmt.Sprintf("%d", int64(result))
		} else {
			text = fmt.Sprintf("%g", result)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: text}},
		}, nil, nil
	})

	// Create in-memory transports and connect.
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	_, err := server.Connect(ctx, serverTransport, nil)
	require.NoError(t, err, "server connect should succeed")

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err, "client connect should succeed")

	cleanup := func() {
		session.Close()
	}

	return session, cleanup
}

// extractText is a helper to extract text content from a CallToolResult.
func extractText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.NotNil(t, result, "result should not be nil")
	require.NotEmpty(t, result.Content, "result should have content")
	tc, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok, "first content should be TextContent")
	return tc.Text
}

func TestMockMCPServer(t *testing.T) {
	ctx := context.Background()
	session, cleanup := setupMockServer(t)
	defer cleanup()

	// List tools and verify 2 tools exist.
	listResult, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	require.NoError(t, err, "ListTools should succeed")
	require.NotNil(t, listResult, "ListTools result should not be nil")
	assert.Len(t, listResult.Tools, 2, "should have 2 tools")

	// Find the tools by name.
	var echoFound, addFound bool
	for _, tool := range listResult.Tools {
		switch tool.Name {
		case "echo":
			echoFound = true
		case "add":
			addFound = true
		}
	}
	assert.True(t, echoFound, "echo tool should be present")
	assert.True(t, addFound, "add tool should be present")

	// Call echo tool with {"message": "hello"}.
	echoResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "echo",
		Arguments: map[string]any{"message": "hello"},
	})
	require.NoError(t, err, "CallTool(echo) should succeed")
	assert.Equal(t, "hello", extractText(t, echoResult), "echo should return the message")

	// Call add tool with {"a": 3, "b": 4}.
	addResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "add",
		Arguments: map[string]any{"a": 3.0, "b": 4.0},
	})
	require.NoError(t, err, "CallTool(add) should succeed")
	assert.Equal(t, "7", extractText(t, addResult), "3 + 4 should equal 7")
}