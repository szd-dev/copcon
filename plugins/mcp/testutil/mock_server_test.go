package testutil_test

import (
	"context"
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
		return &mcp.CallToolResult{
			// Format the number without unnecessary decimals
			Content: []mcp.Content{&mcp.TextContent{Text: formatSum(args.A, args.B, result)}},
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

// formatSum formats a sum result as a string, using integer format when
// both operands are whole numbers.
func formatSum(a, b, result float64) string {
	if a == float64(int64(a)) && b == float64(int64(b)) {
		return formatInt(int64(result))
	}
	return formatFloat(result)
}

func formatInt(n int64) string {
	return string(itoa(n)) // using simplified conversion
}

func formatFloat(f float64) string {
	s := ftoa(f)
	// Trim trailing zeros
	for len(s) > 0 && s[len(s)-1] == '0' {
		s = s[:len(s)-1]
	}
	if len(s) > 0 && s[len(s)-1] == '.' {
		s = s[:len(s)-1]
	}
	return s
}

// Simplified integer-to-string conversion to avoid strconv import.
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// Simplified float-to-string conversion to avoid strconv import.
func ftoa(f float64) string {
	// Handle integer values
	if f == float64(int64(f)) {
		return itoa(int64(f))
	}
	// Simple approach: build from integer and fractional parts
	intPart := int64(f)
	fracPart := f - float64(intPart)
	result := itoa(intPart) + "."
	// Add up to 6 decimal places
	for i := 0; i < 6; i++ {
		fracPart *= 10
		digit := int64(fracPart)
		result += string(byte('0' + digit))
		fracPart -= float64(digit)
		if fracPart == 0 {
			break
		}
	}
	return result
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