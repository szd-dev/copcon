package mcp

import (
	"context"
	"testing"

	gmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractMCPContent(t *testing.T) {
	tests := []struct {
		name   string
		result *gmcp.CallToolResult
		want   string
	}{
		{
			name:   "nil result",
			result: nil,
			want:   "",
		},
		{
			name:   "empty content",
			result: &gmcp.CallToolResult{},
			want:   "",
		},
		{
			name: "single TextContent",
			result: &gmcp.CallToolResult{
				Content: []gmcp.Content{&gmcp.TextContent{Text: "hello"}},
			},
			want: "hello",
		},
		{
			name: "multiple TextContent parts",
			result: &gmcp.CallToolResult{
				Content: []gmcp.Content{
					&gmcp.TextContent{Text: "line1"},
					&gmcp.TextContent{Text: "line2"},
				},
			},
			want: "line1\nline2",
		},
		{
			name: "unsupported content type",
			result: &gmcp.CallToolResult{
				Content: []gmcp.Content{
					&gmcp.TextContent{Text: "text"},
					&gmcp.ImageContent{Data: []byte("fake"), MIMEType: "image/png"},
				},
			},
			want: "text\n[unsupported content type: *mcp.ImageContent]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractMCPContent(tt.result)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildMCPToolCallFunc(t *testing.T) {
	ctx := context.Background()

	server := gmcp.NewServer(&gmcp.Implementation{Name: "test", Version: "1.0.0"}, nil)

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

	client := gmcp.NewClient(&gmcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer session.Close()

	callFunc := buildMCPToolCallFunc(session)

	result, err := callFunc(ctx, "echo", map[string]any{"message": "hello"})
	require.NoError(t, err)
	assert.Equal(t, "hello", result)
}
