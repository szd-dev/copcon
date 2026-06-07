## Learning: Project Architecture

### Module structure
- `go.work` includes: `./core`, `./plugins`, `./server`
- `plugins/go.mod` is a SHARED module for ALL plugins (NOT per-plugin go.mod)
- Plugin packages live under `plugins/` as packages: `memoryfile`, `skill`, `knowledgebase`, etc.
- `plugins/mcp/` will be a package under `github.com/copcon/plugins/mcp`

### Plugin registration pattern
- `RegisterCapabilities(r *capabilities.Registry, ...)` with plugin-specific config args
- Returns void (logs warning on error)
- `r.Register(module)` — module implements `ModuleCapability` interface

### ModuleCapability interface (core/capabilities/registry.go)
```go
type ModuleCapability interface {
    Capability  // Name(), Type(), DependsOn()
    NewHooks(deps CapabilityDeps) ([]hook.Hook, error)
    NewTools(deps CapabilityDeps) ([]tool.Tool, error)
}
```

### CapabilityDeps (core/capabilities/registry.go:72-80)
```go
type CapabilityDeps struct {
    SessionStore, MessageStore, TodoStore storage.*
    AgentRegistry agent.AgentRegistry
    Engine interface{}
    Logger *slog.Logger
    AgentKnowledgeBases map[string][]string
}
```

### Existing capability names
- `capabilities.CapMemoryFile` = string constant in core/capabilities/constants.go
- `skill` module uses "modules.skills"
- MCP should use "modules.mcp"

### Wildcard support
- Registry supports: tools.*, hooks.*, skills.*, memory.*, modules.*, *
- MCP module will be accessible via "modules.mcp" or "modules.*" wildcard

### MCP SDK API (modelcontextprotocol/go-sdk v1.6.1)
- `mcp.NewClient(impl, opts)` → `*mcp.Client`
- `client.Connect(ctx, transport, opts)` → `*mcp.ClientSession, error`
- `session.ListTools(ctx, params)` → `*mcp.ListToolsResult, error`
- `session.CallTool(ctx, params)` → `*mcp.CallToolResult, error`
- Transports: `mcp.CommandTransport`, `mcp.SSEClientTransport`, `mcp.StreamableClientTransport`
- `mcp.NewInMemoryTransport()` for testing

## Architecture Decision: Single shared module
- MCP plugin lives as `plugins/mcp/` package, NOT a separate go.mod
- MCP SDK dependency added to `plugins/go.mod`
- No changes to go.work needed

## Wave 1 Implementation Notes

### Files created
- `plugins/mcp/doc.go` — package documentation
- `plugins/mcp/config.go` — MCPServerConfig, TransportType, AllowedToolsConfig
- `plugins/mcp/types.go` — MCPToolInfo, MCPToolWrapper, naming functions (buildMCPToolName, parseMCPToolName, normalizeName)
- `plugins/mcp/types_test.go` — table-driven tests for naming functions
- `plugins/mcp/testutil/mock_server_test.go` — mock MCP server with echo/add tools, tested via InMemoryTransport

### MCP SDK API (v1.6.1) patterns used
- `mcp.NewServer(impl, opts)` → `*mcp.Server`
- `mcp.NewClient(impl, opts)` → `*mcp.Client`
- `mcp.NewInMemoryTransports()` → `(serverTransport, clientTransport)`
- `server.Connect(ctx, transport, nil)` → `*mcp.ServerSession, error`
- `client.Connect(ctx, transport, nil)` → `*mcp.ClientSession, error`
- `mcp.AddTool(server, tool, handler)` — generic typed tool registration
- `session.ListTools(ctx, params)` → `*mcp.ListToolsResult`
- `session.CallTool(ctx, params)` → `*mcp.CallToolResult`
- `mcp.Tool` struct: Name, Description, InputSchema fields
- `mcp.CallToolResult.Content` → `[]mcp.Content` (type-assert to `*mcp.TextContent` for text)
- `mcp.CallToolParams` has Name (string) and Arguments (any, marshaled to JSON)

### Key decisions
- `normalizeName` strips spaces (doesn't replace with underscore) — only dots are replaced with underscores
- `parseMCPToolName` uses `strings.Cut` for cleaner parsing
- Mock server test uses `testutil_test` package (external test package) to avoid import cycles
- Custom itoa/ftoa helpers used instead of strconv to minimize dependencies

## Wave 2 Implementation Notes

### Files created
- `plugins/mcp/content.go` — `extractMCPContent` (Content→string extraction), `buildMCPToolCallFunc` (session→MCPToolCallFunc)
- `plugins/mcp/content_test.go` — table-driven tests for content extraction + integration test with mock server
- `plugins/mcp/connection.go` — `ConnectionManager` with Connect/Disconnect/GetSession/ListSessions/DisconnectAll + `ConnectWithTransport` for testing + `createTransport` factory
- `plugins/mcp/connection_test.go` — tests for all ConnectionManager methods including concurrent access (race detector) and transport validation
- `plugins/mcp/capability.go` — `MCPModule` implementing `ModuleCapability`, `isToolAllowed` filter, `convertSchema`
- `plugins/mcp/capability_test.go` — tests for NewTools, allowed tools filtering, partial failure, NewHooks returns nil, interface compliance

### SDK Transport types (verified from source)
- Interface: `mcp.Transport` (NOT `ClientTransport` — that doesn't exist)
- `mcp.CommandTransport{Command: *exec.Cmd}` — stdio transport
- `mcp.SSEClientTransport{Endpoint: string, HTTPClient: *http.Client}` — SSE transport
- `mcp.StreamableClientTransport{Endpoint: string, HTTPClient: *http.Client}` — streamable HTTP
- `mcp.InMemoryTransport` — for testing, created via `mcp.NewInMemoryTransports()`

### SDK types for tool discovery
- `Tool.InputSchema` is `any` type (not a struct) — from client side, it marshals to `map[string]any`
- `CallToolParams.Arguments` is `any` (not `map[string]any`) — accepts any JSON-marshalable value
- `ListToolsResult.Tools` is `[]*Tool` (slice of pointers)
- `Content` is an interface with types: `*TextContent`, `*ImageContent`, `*AudioContent`, `*ResourceLink`, `*EmbeddedResource`

### Key design decisions
- `ConnectWithTransport` method on ConnectionManager enables testing without real servers
- `MCPModule.NewTools` does graceful degradation: if a server fails to connect, skip it and continue
- `convertSchema` handles `map[string]any` (the common client-side representation) and falls back to `{"type": "object"}`
- `isToolAllowed` implements include/exclude filtering where exclude wins over include
- Tests use `package mcp` (internal tests) for access to unexported functions like `createTransport`, `isToolAllowed`

## Wave 3 Implementation Notes

### Files created
- `plugins/mcp/register.go` — `CapabilityName` constant + `RegisterCapabilities` function
- `plugins/mcp/register_test.go` — tests for registration, empty configs, constant value
- `plugins/mcp/integration_test.go` — end-to-end test (server→ConnectionManager→MCPModule→tool discovery→tool execution) + MCP prefix verification test

### Files modified
- `server/internal/config/config.go` — added MCPConfig, MCPServerConfig, MCPAllowedTools structs + MCP field to Config
- `server/cmd/server/main.go` — added mcp import, MCP registration block, convertMCPServerConfigs helper, MCP capability in agentSpecs
- `server/config.yaml` — added commented MCP config section as example

### Key patterns
- `capabilities.Registry.Get(name)` returns `(Capability, bool)` NOT `(Capability, error)` — different from typical Go patterns
- Server config types are SEPARATE from plugin config types: server uses `string` for Type (YAML-friendly), plugin uses `TransportType` enum. Conversion function bridges them.
- `RegisterCapabilities` pattern: `if cfg.Feature.Enabled { pkg.RegisterCapabilities(reg, ...) }` — consistent with skill plugin
- Agent tool list building: append capability name constant to tools slice in `agentSpecs()`
- `iface.ChatContextInterface` mock only needs `Context() context.Context` for MCPToolWrapper.Execute — other methods can be zero-valued
- Integration test uses `mockChatContext` struct implementing full `iface.ChatContextInterface` with proper types from `core/entity` and `core/iface`
