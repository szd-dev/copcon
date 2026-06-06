package mcp

// TransportType identifies the MCP transport protocol.
type TransportType string

const (
	// TransportStdio runs the MCP server as a subprocess communicating over
	// stdin/stdout.
	TransportStdio TransportType = "stdio"

	// TransportSSE communicates with the MCP server over Server-Sent Events.
	TransportSSE TransportType = "sse"

	// TransportStreamableHTTP communicates with the MCP server over a
	// streamable HTTP transport.
	TransportStreamableHTTP TransportType = "streamable-http"
)

// MCPServerConfig describes how to connect to a single MCP server.
type MCPServerConfig struct {
	// Name is a human-readable identifier for this MCP server.
	Name string `json:"name" yaml:"name"`

	// Type selects the transport protocol.
	Type TransportType `json:"type" yaml:"type"`

	// Command is the executable path for stdio transport.
	Command string `json:"command,omitempty" yaml:"command,omitempty"`

	// Args holds command-line arguments for stdio transport.
	Args []string `json:"args,omitempty" yaml:"args,omitempty"`

	// Env holds additional environment variables for stdio transport.
	Env map[string]string `json:"env,omitempty" yaml:"env,omitempty"`

	// URL is the endpoint URL for SSE / StreamableHTTP transport.
	URL string `json:"url,omitempty" yaml:"url,omitempty"`

	// AllowedTools controls which tools from this server are exposed.
	AllowedTools *AllowedToolsConfig `json:"allowed_tools,omitempty" yaml:"allowed_tools,omitempty"`
}

// AllowedToolsConfig controls which MCP tools from a server are exposed
// to the agent. An empty/nil config means all tools are allowed.
type AllowedToolsConfig struct {
	// Include is an explicit allow-list of tool names. If non-empty, only
	// tools whose names appear in this list are exposed.
	Include []string `json:"include,omitempty" yaml:"include,omitempty"`

	// Exclude is a deny-list of tool names. Tools whose names appear here
	// are never exposed, even if they appear in Include.
	Exclude []string `json:"exclude,omitempty" yaml:"exclude,omitempty"`
}