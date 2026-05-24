# Go API Reference

Complete reference for the `github.com/copcon/core` library. The core module is a standalone Go library you can embed in any Go application to get a full agent engine with tool execution, hook lifecycle, streaming, and persistent storage.

## Package Overview

| Package | Purpose |
|---------|---------|
| `github.com/copcon/core` | Top-level Harness, `NewAgent`, `APIProvider` |
| `github.com/copcon/core/agent` | Agent engine, registry, context building |
| `github.com/copcon/core/storage` | Storage interfaces and value types |
| `github.com/copcon/core/llm` | LLM provider abstraction |
| `github.com/copcon/core/tool` | Tool registration, management, async tracking |
| `github.com/copcon/core/hook` | Hook lifecycle and runner |
| `github.com/copcon/core/chat` | SSE chat handler, active session store |
| `github.com/copcon/core/chatcontext` | ChatContext implementation |
| `github.com/copcon/core/iface` | Core interfaces (`ChatContextInterface`, `Storer`) |
| `github.com/copcon/core/entity` | Shared value types (events, messages, UI models) |
| `github.com/copcon/core/context_builder` | Context window assembly for LLM calls |
| `github.com/copcon/core/capabilities` | Capability registry, dependency resolution |
| `github.com/copcon/core/providers/postgres` | PostgreSQL storage implementation |
| `github.com/copcon/core/providers/qdrant` | Qdrant vector memory implementation |

---

## Quick Start

### Minimal Single Agent

```go
package main

import (
    "context"
    "fmt"
    "log/slog"
    "os"

    "github.com/copcon/core"
    "github.com/copcon/core/llm"
    pgstore "github.com/copcon/core/providers/postgres"
    "gorm.io/driver/postgres"
    "gorm.io/gorm"
    "github.com/openai/openai-go/v3"
    "github.com/openai/openai-go/v3/option"
)

func main() {
    log := slog.New(slog.NewTextHandler(os.Stderr, nil))

    db, _ := gorm.Open(postgres.Open("host=localhost port=5432 user=admin password=changeme dbname=copcon sslmode=disable"), &gorm.Config{})
    pg := pgstore.NewStore(db)

    cl := openai.NewClient(option.WithAPIKey("sk-..."))
    provider := llm.NewOpenAIAdapter(&cl, "gpt-4o")

    engine, registry, err := core.NewAgent(core.AgentQuickConfig{
        Name:         "Assistant",
        Model:        "gpt-4o",
        SystemPrompt: "You are a helpful assistant.",
        Tools:        []string{"code_executor", "shell_executor"},
        LLM:          provider,
        SessionStore: pg.Sessions(),
        MessageStore: pg.Messages(),
    })
    if err != nil {
        log.Error("failed to create agent", "error", err)
        os.Exit(1)
    }

    _ = engine
    _ = registry
    fmt.Println("Agent created successfully")
}
```

---

## core (Top-Level Package)

### `AgentQuickConfig`

Simplified configuration for single-agent setups. Wraps `HarnessConfig` and calls `Build()` for you.

```go
type AgentQuickConfig struct {
    Name         string           // Agent display name
    Model        string           // LLM model (e.g., "gpt-4o")
    SystemPrompt string           // System prompt text
    Tools        []string         // Tool capability names (e.g., "code_executor")
    Hooks        []string         // Hook capability names
    LLM          llm.LLMProvider  // LLM provider instance
    SessionStore storage.SessionStore
    MessageStore storage.MessageStore
}
```

### `NewAgent`

Creates a single-agent Harness, calls `Build()`, and returns the engine and registry.

```go
func NewAgent(cfg AgentQuickConfig) (agent.AgentEngine, agent.AgentRegistry, error)
```

**Returns:**
- `AgentEngine` for processing chat requests
- `AgentRegistry` for querying agent metadata
- `error` if `Build()` fails

### `HarnessConfig`

Full configuration for multi-agent setups.

```go
type HarnessConfig struct {
    Store          StoreConfig           // Storage providers (required)
    LLM            llm.LLMProvider       // Default LLM provider
    Logger         *slog.Logger          // Structured logger
    Agents         []AgentSpec           // Static agent declarations
    AgentFactories []AgentFactorySpec    // Dynamic agent factories
    AsyncTracker   tool.AsyncToolTracker // Override async tool tracker
}
```

### `StoreConfig`

```go
type StoreConfig struct {
    Provider storage.StoreProvider // Required: implements Sessions(), Messages(), Todos()
    Memory   storage.MemoryStore   // Optional: nil skips memory hook registration
}
```

### `AgentSpec`

Declares a static agent. During `Build()`, each spec is auto-converted to an `AgentFactory`.

```go
type AgentSpec struct {
    ID            string   // Unique agent identifier
    Name          string   // Display name
    Model         string   // LLM model override
    SystemPrompt  string   // System prompt template
    Tools         []string // Tool capability names
    AllowDelegate bool     // Allow other agents to delegate to this agent
}
```

### `AgentFactorySpec`

Declares a dynamic agent factory. Use this when you need runtime control over agent creation.

```go
type AgentFactorySpec struct {
    ID            string              // Unique identifier
    Name          string              // Display name
    Model         string              // Default model
    Factory       agent.AgentFactory  // Factory function
    AllowDelegate bool                // Allow delegation
}
```

### `Harness`

The main orchestrator. Collects capabilities, wires dependencies, creates the engine.

```go
type Harness struct { /* internal fields */ }

func NewHarness(cfg HarnessConfig) *Harness
func (h *Harness) Build() error
func (h *Harness) Engine() agent.AgentEngine
func (h *Harness) Registry() agent.AgentRegistry
func (h *Harness) AsyncTracker() tool.AsyncToolTracker
func (h *Harness) Store() storage.StoreProvider
func (h *Harness) ActiveSessions() chat.ActiveSessions
```

**`Build()` lifecycle:**

1. Validate `StoreConfig.Provider` (required, must not be nil)
2. Collect capability names from agent specs
3. Resolve dependencies (topological sort)
4. Create `ToolRegistry` and register tools from resolved capabilities
5. Create `HookRunner` and register hooks (skip `hooks.memory` if `MemoryStore` is nil)
6. Create `AgentRegistry` with the first agent as default
7. Register `AgentSpec` entries as factories
8. Register `AgentFactorySpec` entries directly
9. Create `AgentEngine`
10. Register cross-agent tools (`delegate_to`, `read_sub_session`)

### `APIProvider`

Interface implemented by `Harness`. Server applications depend on this.

```go
type APIProvider interface {
    Store() storage.StoreProvider
    Engine() agent.AgentEngine
    Registry() agent.AgentRegistry
    ActiveSessions() chat.ActiveSessions
}
```

### Built-In Capabilities

These are automatically registered when you import the capabilities packages via blank imports in `harness.go`:

```go
import (
    _ "github.com/copcon/core/capabilities/hooks"  // logging, tracing, memory, todo_injection
    _ "github.com/copcon/core/capabilities/tools"   // ask_user, confirm_action, todo, async
)
```

**Built-in hooks:** `hooks.todo_injection`, `hooks.memory`, `hooks.logging`, `hooks.tracing`

**Built-in tools:** `tools.confirm_action`, `tools.ask_user`, `tools.todo`, `tools.async`

**Optional tools** (specify in `AgentSpec.Tools`): `code_executor`, `shell_executor`, `file_ops`

---

## agent Package

### `AgentEngine`

The core interface. Processes user input through the agent loop (LLM call, tool execution, repeat).

```go
type AgentEngine interface {
    Chat(chatCtx iface.ChatContextInterface, userInput string) error
}
```

`Chat` runs the full agent loop. It emits events via `chatCtx.Emit()`, which the caller can subscribe to. Returns `nil` even if the agent loop encounters an error (the error is emitted as an `error` event). Returns a non-nil error only if `chatCtx.Depth() >= 3` (max subagent depth exceeded).

### `NewAgentEngine`

```go
func NewAgentEngine(
    agentRegistry  AgentRegistry,
    sessionStore   storage.SessionStore,
    messageStore   storage.MessageStore,
    ctxBuilder     context_builder.ContextBuilder,
    asyncRegistry  tool.AsyncToolTracker,
    opts           ...EngineOption,
) AgentEngine
```

### `EngineOption`

```go
func WithConcurrency(n int) EngineOption          // Max parallel tool executions (default: 5)
func WithLogger(logger *slog.Logger) EngineOption  // Custom logger
func WithHookRunner(runner hook.HookRunner) EngineOption
func WithGlobalHooks(hooks ...hook.Hook) EngineOption
func WithLLMProvider(p llm.LLMProvider) EngineOption  // Override per-agent LLM provider
```

### Agent Loop Behavior

The engine loops through these steps, up to `maxSteps` (50) iterations:

1. Resolve agent definition from registry
2. Persist user message to `MessageStore`
3. Run hooks at `OnSystemPrompt`, `BeforeContextBuild`
4. Build context window from message history
5. Run hooks at `AfterContextBuild`, `BeforeLLMCall`
6. Stream LLM response, emitting `part_create`/`part_update` events
7. Checkpoint streaming parts every 10 deltas (incremental persistence)
8. Run hooks at `AfterLLMCall`
9. Execute tool calls (if any), emit tool results
10. If tool calls were executed, loop back to step 3

### `AgentRegistry`

```go
type AgentRegistry interface {
    Get(id string) (AgentDefinition, error)
    List() []AgentInfo
    Default() (AgentDefinition, error)
    RegisterFactory(id, name, model string, allowDelegate bool, factory AgentFactory)
    GetFactory(id string) (AgentFactory, error)
    ListDelegatable() []AgentInfo
}

func NewAgentRegistry(defaultAgentID string) AgentRegistry
```

### `AgentFactory`

```go
type AgentFactory func(ctx context.Context, params CreateParams) (AgentDefinition, error)

type CreateParams struct {
    Task          string         // Task description for delegation
    ParentContext string         // Parent session context
    ModelOverride string         // Override the default model
    Extra         map[string]any // Additional parameters
}
```

### `AgentDefinition`

Produced by an `AgentFactory`. Fully describes an agent ready to run.

```go
type AgentDefinition struct {
    ID           string
    Name         string
    Model        string
    SystemPrompt string
    ToolManager  tool.ToolManager
    LLMProvider  llm.LLMProvider
    Hooks        []hook.Hook
}
```

### `AgentInfo`

Lightweight metadata returned by `List()`.

```go
type AgentInfo struct {
    ID    string
    Name  string
    Model string
}
```

### `StreamResult`

Internal result type from a single LLM call step.

```go
type StreamResult struct {
    MessageID        string
    StepIndex        int
    Content          string
    ReasoningContent string
    ToolCalls        []toolCallInfo
    ToolResults      map[string]*ToolCallResult
    Usage            *llm.Usage
}
```

---

## storage Package

All storage types are pure value objects with no GORM annotations. The `providers/postgres` package handles GORM mapping.

### `StoreProvider`

Aggregates all storage interfaces.

```go
type StoreProvider interface {
    Sessions() SessionStore
    Messages() MessageStore
    Todos()    TodoStore
}
```

### `SessionStore`

```go
type SessionStore interface {
    Create(ctx context.Context, session *Session) (*Session, error)
    Get(ctx context.Context, id uuid.UUID) (*Session, error)
    List(ctx context.Context, limit, offset int) ([]*Session, int64, error)
    Delete(ctx context.Context, id uuid.UUID) error
    UpdateTitle(ctx context.Context, id uuid.UUID, title string) error
    UpdateMetadata(ctx context.Context, id uuid.UUID, metadata map[string]any) error
    GetMessageCount(ctx context.Context, sessionID uuid.UUID) (int64, error)
    AppendMetadata(ctx context.Context, id uuid.UUID, key string, value any) error
}
```

### `Session`

```go
type Session struct {
    ID              uuid.UUID
    Title           string
    DefaultAgentID  string
    ParentSessionID *uuid.UUID
    Metadata        map[string]any
    CreatedAt       time.Time
    UpdatedAt       time.Time
}
```

### `MessageStore`

```go
type MessageStore interface {
    List(ctx context.Context, sessionID uuid.UUID, limit int) ([]*Message, error)
    Add(ctx context.Context, message *Message) error
    Update(ctx context.Context, message *Message) error
    Upsert(ctx context.Context, message *Message) error
    DeleteBySession(ctx context.Context, sessionID uuid.UUID) error
}
```

### `Message`

```go
type Message struct {
    ID         uuid.UUID
    SessionID  uuid.UUID
    Role       string        // "user", "assistant", "tool"
    Content    string
    Reasoning  string
    ToolCalls  []ToolCall
    ToolCallID string        // For "tool" role messages
    Parts      []Part
    Model      string
    TokenCount int
    DurationMs int64
    CreatedAt  time.Time
}
```

### `Part`

```go
type Part struct {
    Type       string  // "text", "reasoning", "tool-call"
    Text       string
    State      string  // "streaming", "done", "pending", "running", "complete", "error", "waiting_for_input"
    ToolCallID string
    ToolName   string
    Args       string
    Output     string
    Error      string
    Interrupt  any
    StepIndex  int
}
```

### `ToolCall`, `FunctionCall`

```go
type ToolCall struct {
    ID       string
    Type     string        // Always "function"
    Function FunctionCall
}

type FunctionCall struct {
    Name      string
    Arguments string  // JSON-encoded
}
```

### `TodoStore`

```go
type TodoStore interface {
    Create(ctx context.Context, todo *Todo) (*Todo, error)
    Get(ctx context.Context, id uuid.UUID) (*Todo, error)
    List(ctx context.Context, sessionID uuid.UUID) ([]*Todo, error)
    UpdateStatus(ctx context.Context, id uuid.UUID, status TodoStatus) (*Todo, error)
    DeleteBySession(ctx context.Context, sessionID uuid.UUID) error
}
```

### `Todo`

```go
type Todo struct {
    ID          uuid.UUID
    SessionID   uuid.UUID
    Content     string
    ActiveForm  string
    Status      TodoStatus  // "pending", "in_progress", "completed", "blocked", "failed"
    Priority    string
    DependsOn   []uuid.UUID
    Validation  string
    Result      string
    RetryCount  int
    CompletedAt *time.Time
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

### `MemoryStore`

```go
type MemoryStore interface {
    Store(ctx context.Context, memory *Memory) error
    Search(ctx context.Context, query []float32, limit int) ([]*Memory, error)
    GetBySession(ctx context.Context, sessionID string, limit int) ([]*Memory, error)
    DeleteBySession(ctx context.Context, sessionID string) error
}
```

### `Memory`

```go
type Memory struct {
    ID         string
    Content    string
    SessionID  string
    Role       string
    Timestamp  time.Time
    MemoryType string
    Metadata   map[string]any
    Score      float32
}
```

---

## llm Package

### `LLMProvider`

```go
type LLMProvider interface {
    Stream(ctx context.Context, params StreamParams) (<-chan StreamChunk, <-chan error)
}
```

The caller must read from the data channel (`ch`) until it closes. The error channel (`errc`) receives at most one error.

### `StreamParams`

```go
type StreamParams struct {
    Model       string     // e.g., "gpt-4o"
    Messages    []Message
    Tools       []ToolDef
    Temperature float64    // 0.0-2.0, 0 means provider default
    MaxTokens   int        // 0 means no explicit limit
}
```

### `StreamChunk`

```go
type StreamChunk struct {
    Content          string           // Text delta
    ReasoningContent string           // Reasoning/thinking delta (DeepSeek, etc.)
    ToolCalls        []ToolCallDelta  // Incremental tool call data
    Usage            *Usage           // Token stats (usually in final chunk)
    FinishReason     string           // "stop", "length", "tool_calls"
}
```

### `Message`

```go
type Message struct {
    Role       StreamRole  // "system", "user", "assistant", "tool"
    Content    string
    ToolCalls  []ToolCall
    ToolCallID string      // For "tool" role
    Name       string      // Optional participant name
}
```

### `ToolDef`

```go
type ToolDef struct {
    Name        string
    Description string
    Parameters  json.RawMessage  // JSON Schema
}
```

### `Usage`

```go
type Usage struct {
    PromptTokens     int64
    CompletionTokens int64
    TotalTokens      int64
}
```

### `OpenAIAdapter`

Built-in adapter for OpenAI-compatible APIs.

```go
func NewOpenAIAdapter(client *openai.Client, model string) *OpenAIAdapter
```

Supports:
- Standard OpenAI chat completions
- DeepSeek `reasoning_content` field
- Parallel tool calls
- Streaming with delta accumulation

---

## tool Package

### `Tool`

```go
type Tool interface {
    Name() string
    Description() string
    InputSchema() map[string]any
    Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*ToolResult, error)
}
```

### `DelegationTool`

Marks tools that delegate to sub-agents. These are excluded from automatic `execution_mode` parameter injection.

```go
type DelegationTool interface {
    IsDelegationTool() bool
}
```

### `ToolResult`

```go
type ToolResult struct {
    Success bool
    Data    any
    Error   string
}
```

### `ToolManager`

Per-agent tool manager. Handles registration, execution, and LLM tool definition generation.

```go
type ToolManager interface {
    Register(tool Tool) error
    Unregister(name string) error
    Get(name string) (Tool, error)
    List() []ToolInfo
    Execute(chatCtx iface.ChatContextInterface, name string, args map[string]any) (*ToolResult, error)
    GetToolDefs() []llm.ToolDef
}

func NewToolManager() ToolManager
```

`GetToolDefs()` automatically injects an `execution_mode` parameter into each tool's schema (unless the tool implements `DelegationTool`):

```json
{
  "execution_mode": {
    "type": "string",
    "enum": ["sync", "concurrent", "async"],
    "default": "sync",
    "description": "Execution mode: 'sync' (wait for result), 'concurrent' (parallel with other tools), 'async' (background execution). Default: sync."
  }
}
```

### `ToolRegistry`

Global registry for all tools. Used by the Harness to create agent-specific `ToolManager` instances.

```go
type ToolRegistry interface {
    Register(tool Tool) error
    Get(name string) (Tool, error)
    List() []ToolInfo
}

func NewToolRegistry() ToolRegistry
```

### `AsyncToolTracker`

Tracks async tool executions across sessions.

```go
type AsyncToolTracker interface {
    Register(sessionID, callID, toolName string, cancelFunc context.CancelFunc)
    Unregister(callID string)
    Complete(callID string, result any)
    Fail(callID string, errMsg string)
    GetStatus(callID string) (*AsyncToolState, error)
    Cancel(callID string) bool
    CancelSession(sessionID string) int
    ListBySession(sessionID string) []*AsyncToolState
}
```

### `AsyncToolState`

```go
type AsyncToolState struct {
    CallID     string
    ToolName   string
    Status     AsyncToolStatus  // "running", "completed", "failed", "cancelled"
    StartTime  time.Time
    EndTime    time.Time
    Result     any
    Error      string
    SessionID  string
    CancelFunc context.CancelFunc
}
```

### Helper Functions

```go
func ParseArguments(argsJSON string) (map[string]any, error)
func ToArgumentsJSON(args map[string]any) string
```

---

## hook Package

### `Hook`

```go
type Hook interface {
    Name() string
    Points() []HookPoint
    Priority() int      // Lower = runs first. Default: 100
    Execute(ctx *HookContext) error
}
```

Hook errors and panics are contained. A failing hook never aborts the pipeline; errors are logged and the chain continues.

### `HookPoint`

```go
type HookPoint string

const (
    BeforeContextBuild HookPoint = "before_context_build"
    AfterContextBuild  HookPoint = "after_context_build"
    OnSystemPrompt     HookPoint = "on_system_prompt"
    OnMessagePersist   HookPoint = "on_message_persist"
    BeforeToolExecute  HookPoint = "before_tool_execute"
    AfterToolExecute   HookPoint = "after_tool_execute"
    OnToolError        HookPoint = "on_tool_error"
    BeforeLLMCall      HookPoint = "before_llm_call"
    AfterLLMCall       HookPoint = "after_llm_call"
    OnSessionResolve   HookPoint = "on_session_resolve"
)
```

### `HookContext`

```go
type HookContext struct {
    ChatCtx      iface.ChatContextInterface
    SessionID    string
    AgentID      string
    SystemPrompt *string                      // Mutable: hooks can replace
    Messages     *[]entity.MessageForLLM      // Mutable: hooks can modify
    ToolName     string
    ToolArgs     map[string]any               // Mutable at BeforeToolExecute
    ToolResult   *tool.ToolResult             // Mutable at AfterToolExecute, OnToolError
    Logger       *slog.Logger
    CurrentPoint HookPoint
}
```

Pointer fields (`*string`, `*[]MessageForLLM`, `*ToolResult`) indicate mutable values that hooks can modify to change downstream engine behavior.

### `HookRunner`

```go
type HookRunner interface {
    Register(hook Hook)
    Run(point HookPoint, ctx *HookContext)
    On(point HookPoint, chatCtx iface.ChatContextInterface, logger *slog.Logger, extra ...HookExtra)
}

func NewHookRunner() HookRunner
func NewEmptyRunner() HookRunner
```

Hooks are sorted by priority (descending), then by registration time (ascending for ties). Context cancellation skips the entire chain.

### `HookExtra`

Optional fields that vary between hook invocation sites.

```go
type HookExtra struct {
    ToolName     *string
    ToolArgs     map[string]any
    ToolResult   *tool.ToolResult
    SystemPrompt *string
    Messages     *[]entity.MessageForLLM
}
```

---

## chat Package

### `HandleChat`

Framework-agnostic SSE chat handler. Works with any `io.Writer` + `http.Flusher`.

```go
func HandleChat(
    ctx context.Context,
    w io.Writer,
    flusher http.Flusher,
    req ChatRequest,
    engine agent.AgentEngine,
    store ActiveSessions,
) error
```

Handles both new chat requests and reconnection. For reconnection, set `req.Reconnect = true` and provide `req.LastEventSeq`.

### `ChatRequest`

```go
type ChatRequest struct {
    SessionID    string `json:"-"`
    Content      string `json:"content"`
    AgentID      string `json:"agent_id"`
    Reconnect    bool   `json:"reconnect"`
    LastEventSeq int64  `json:"last_event_seq"`
}
```

### `ActiveSessions`

Thread-safe store for active chat contexts.

```go
type ActiveSessions interface {
    Get(sessionID string) (*chatcontext.ChatContext, bool)
    Put(sessionID string, chatCtx *chatcontext.ChatContext)
    Remove(sessionID string)
}

func NewActiveSessions() ActiveSessions
```

---

## chatcontext Package

### `ChatContext`

Concrete implementation of `iface.ChatContextInterface`. Encapsulates session identity, event streaming via a ring buffer, and human-in-the-loop interrupt handling.

```go
type ChatContext struct { /* internal fields */ }

func NewChatContext(ctx context.Context, sessionID, agentID string) *ChatContext
func (c *ChatContext) WithDepth(d int) *ChatContext
```

**Key methods:**

| Method | Description |
|--------|-------------|
| `Context()` | Returns the lifecycle context |
| `SessionID()` | Returns the session UUID string |
| `AgentID()` | Returns the agent identifier |
| `Emit(event)` | Writes event to ring buffer, increments sequence |
| `Events()` | Returns a read-only event channel (full buffer) |
| `Subscribe(fromSeq)` | Returns events from a specific sequence number |
| `Close()` | Cancels context, closes buffer, removes from store |
| `Closed()` | Returns channel that fires once after `Close()` |
| `Depth()` | Returns nesting depth for sub-agent calls |
| `RequestInput(req)` | Blocks until human responds (HITL) |
| `ResolveInput(id, resp)` | Resolves a pending interrupt |
| `PendingInputs()` | Lists unresolved interrupts |
| `SetPartLocator(...)` | Stores part location for interrupt events |
| `ClearPartLocator()` | Removes part location |

### Reconnection

`Subscribe(fromSeq)` returns events starting from a given sequence number. If the sequence has been evicted from the ring buffer (size 1024), it returns `(nil, false)`.

---

## iface Package

### `ChatContextInterface`

```go
type ChatContextInterface interface {
    Context() context.Context
    SessionID() string
    AgentID() string
    Events() <-chan entity.Event
    Emit(event entity.Event)
    Close()
    Closed() <-chan struct{}
    Depth() int
    Subscribe(fromSeq int64) (*Subscriber, bool)
    RequestInput(req InputRequest) (*InputResponse, error)
    ResolveInput(interruptID string, resp *InputResponse) error
    PendingInputs() []InputRequest
    SetPartLocator(messageID string, stepIndex, partIndex int)
    ClearPartLocator()
}
```

### `InputRequest`

```go
type InputRequest struct {
    ID          string         `json:"id"`
    Type        InterruptType  `json:"type"`  // "approval" or "question"
    Message     string         `json:"message"`
    InputSchema map[string]any `json:"input_schema,omitempty"`
    Summary     string         `json:"summary,omitempty"`
    ToolName    string         `json:"tool_name,omitempty"`
    ToolArgs    map[string]any `json:"tool_args,omitempty"`
}
```

### `InputResponse`

```go
type InputResponse struct {
    Action  string         `json:"action"`
    Content map[string]any `json:"content,omitempty"`
}
```

---

## entity Package

### Event Types

```go
// Current (step/part model)
EventStepCreate    EventType = "step_create"
EventPartCreate    EventType = "part_create"
EventPartUpdate    EventType = "part_update"
EventMessageDone   EventType = "message_done"

// Async tool events
EventAsyncToolStarted  EventType = "async_tool_started"
EventAsyncToolComplete EventType = "async_tool_complete"
EventAsyncToolFailed   EventType = "async_tool_failed"

// Always active
EventError EventType = "error"

// Legacy (deprecated, kept for backward compatibility)
EventMessage    EventType = "message"
EventReasoning  EventType = "reasoning"
EventToolCall   EventType = "tool_call"
EventToolResult EventType = "tool_result"
EventDone       EventType = "done"
```

### `Event`

```go
type Event struct {
    Type EventType `json:"type"`
    Data any       `json:"data"`
}
```

### Event Data Types

| Event | Data Type | Key Fields |
|-------|-----------|------------|
| `step_create` | `StepCreateData` | `messageId`, `stepIndex` |
| `part_create` | `PartCreateData` | `messageId`, `stepIndex`, `partIndex`, `partType`, `state` |
| `part_update` | `PartUpdateData` | `messageId`, `stepIndex`, `partIndex`, `partType`, `textDelta`, `state`, `output`, `error`, `interrupt` |
| `message_done` | `MessageDoneData` | `messageId` |
| `error` | `ErrorData` | `error` |
| `async_tool_started` | `AsyncToolStartedData` | `message_id`, `call_id`, `tool_name`, `session_id` |
| `async_tool_complete` | `AsyncToolCompleteData` | `message_id`, `call_id`, `tool_name`, `result`, `duration_ms` |
| `async_tool_failed` | `AsyncToolFailedData` | `message_id`, `call_id`, `tool_name`, `error`, `duration_ms` |

### `UIMessage`

```go
type UIMessage struct {
    ID        string     `json:"id"`
    SessionID string     `json:"session_id"`
    Role      string     `json:"role"`  // "user" or "assistant"
    Steps     []UIStep   `json:"steps"`
    Parts     []UIPart   `json:"parts"`  // Deprecated: use Steps
    Metadata  UIMetadata `json:"metadata"`
}
```

### `UIStep`

```go
type UIStep struct {
    Parts []UIPart    `json:"parts"`
    State UIPartState `json:"state"`
}
```

### `UIPart`

```go
type UIPart struct {
    Type       UIPartType     `json:"type"`       // "text", "reasoning", "tool-call", "step-start"
    StepIndex  int            `json:"stepIndex,omitempty"`
    Text       string         `json:"text,omitempty"`
    State      UIPartState    `json:"state,omitempty"`
    ToolCallID string         `json:"toolCallId,omitempty"`
    ToolName   string         `json:"toolName,omitempty"`
    Args       string         `json:"args,omitempty"`
    Output     string         `json:"output,omitempty"`
    Error      string         `json:"error,omitempty"`
    Interrupt  map[string]any `json:"interrupt,omitempty"`
}
```

### Part States

```go
UIPartStateStreaming       UIPartState = "streaming"
UIPartStateDone            UIPartState = "done"
UIPartStatePending         UIPartState = "pending"
UIPartStateRunning         UIPartState = "running"
UIPartStateComplete        UIPartState = "complete"
UIPartStateError           UIPartState = "error"
UIPartStateWaitingForInput UIPartState = "waiting_for_input"
```

---

## capabilities Package

### `Capability`

```go
type Capability interface {
    Name() string
    Type() CapabilityType  // "tool", "hook", "skill", "memory"
    DependsOn() []string
}
```

### `ToolCapability`

```go
type ToolCapability interface {
    Capability
    NewTool(deps CapabilityDeps) (tool.Tool, error)
}
```

### `HookCapability`

```go
type HookCapability interface {
    Capability
    NewHook(deps CapabilityDeps) (hook.Hook, error)
}
```

### `CapabilityDeps`

```go
type CapabilityDeps struct {
    SessionStore  storage.SessionStore
    MessageStore  storage.MessageStore
    TodoStore     storage.TodoStore
    MemoryStore   storage.MemoryStore
    AgentRegistry agent.AgentRegistry
    Engine        interface{}
    Logger        *slog.Logger
}
```

### Global Registry Functions

```go
func Register(c Capability)                                      // Thread-safe, call from init()
func Get(name string) (Capability, bool)                         // Retrieve by name
func ListByType(t CapabilityType) []Capability                   // All caps of a type
func ExpandWildcards(names []string) []string                    // Expand "tools.*", "hooks.*", "*"
func ResolveDependencies(names []string) ([]Capability, error)   // Topological sort with deps
```

### Wildcard Patterns

| Pattern | Expansion |
|---------|-----------|
| `tools.*` | All registered tool capabilities |
| `hooks.*` | All registered hook capabilities |
| `skills.*` | All registered skill capabilities |
| `memory.*` | All registered memory capabilities |
| `*` | All registered capabilities |

---

## context_builder Package

### `ContextBuilder`

```go
type ContextBuilder interface {
    Build(ctx context.Context, messages []entity.UIMessage, systemPrompt string, userInput string) ([]entity.MessageForLLM, error)
}

func New() ContextBuilder
```

Converts `UIMessage` structures into flat `MessageForLLM` sequences for LLM providers. Handles tool call/result pairing and step flattening.

---

## Common Patterns

### Multi-Agent Setup

```go
h := core.NewHarness(core.HarnessConfig{
    Store: core.StoreConfig{Provider: pg},
    LLM:   provider,
    Agents: []core.AgentSpec{
        {ID: "code-assistant", Name: "Code Assistant", Model: "gpt-4o",
         SystemPrompt: "You write code.", Tools: []string{"code_executor", "shell_executor"},
         AllowDelegate: true},
        {ID: "reviewer", Name: "Code Reviewer", Model: "gpt-4o",
         SystemPrompt: "You review code.", Tools: []string{"file_ops"}},
    },
})
```

### Custom Tool Registration

```go
type myTool struct{}

func (t *myTool) Name() string                { return "my_tool" }
func (t *myTool) Description() string         { return "Does something custom" }
func (t *myTool) InputSchema() map[string]any {
    return map[string]any{
        "type": "object",
        "properties": map[string]any{
            "input": map[string]any{"type": "string"},
        },
        "required": []string{"input"},
    }
}
func (t *myTool) Execute(chatCtx iface.ChatContextInterface, args map[string]any) (*tool.ToolResult, error) {
    input, _ := args["input"].(string)
    return &tool.ToolResult{Success: true, Data: "processed: " + input}, nil
}
```

### Custom Hook

```go
type myHook struct{}

func (h *myHook) Name() string        { return "my_hook" }
func (h *myHook) Points() []hook.HookPoint {
    return []hook.HookPoint{hook.BeforeLLMCall, hook.AfterLLMCall}
}
func (h *myHook) Priority() int { return 50 }  // Runs before default-priority hooks
func (h *myHook) Execute(ctx *hook.HookContext) error {
    if ctx.CurrentPoint == hook.BeforeLLMCall {
        // Modify messages before sending to LLM
        *ctx.Messages = append(*ctx.Messages, entity.MessageForLLM{
            Role: "system", Content: "Extra context injected by hook",
        })
    }
    return nil
}
```

### Processing Events

```go
chatCtx := chatcontext.NewChatContext(ctx, sessionID, "code-assistant")
go func() {
    defer chatCtx.Close()
    engine.Chat(chatCtx, "Write a hello world function")
}()

for event := range chatCtx.Events() {
    switch event.Type {
    case entity.EventPartCreate:
        data := event.Data.(entity.PartCreateData)
        fmt.Printf("New %s part at step %d\n", data.PartType, data.StepIndex)
    case entity.EventPartUpdate:
        data := event.Data.(entity.PartUpdateData)
        if data.TextDelta != "" {
            fmt.Print(data.TextDelta)
        }
    case entity.EventMessageDone:
        fmt.Println("\nDone.")
    case entity.EventError:
        data := event.Data.(entity.ErrorData)
        fmt.Fprintf(os.Stderr, "Error: %s\n", data.Error)
    }
}
```
