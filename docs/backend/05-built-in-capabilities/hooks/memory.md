# Memory Hook

The memory hook gives agents long-term recall across sessions. It retrieves relevant memories from a vector store before each LLM call and persists assistant responses as new memories after they're sent.

This is how CopCon agents remember past conversations, not just the current session's message history.

## Purpose

Without the memory hook, an agent's knowledge is limited to the messages in the current session. Once the session ends, that context is gone. The memory hook solves this by:

1. **Retrieval**: Before an LLM call, searching the vector store for memories semantically related to the user's latest message and injecting them as a system message.
2. **Persistence**: After a message is persisted, storing the assistant's response in the vector store so it can be found in future sessions.

## Hook Points

| Hook Point | What Happens |
|------------|-------------|
| `AfterContextBuild` | Searches the vector store for memories related to the last user message. Prepends a system message with the results. |
| `OnMessagePersist` | Stores the last assistant message as a new memory entry (asynchronously). |

Priority: **100** (runs before logging and tracing hooks).

## How It Works

### Retrieval (AfterContextBuild)

1. The hook finds the last user message in the assembled message list.
2. It encodes the message text into a vector (currently a byte-encoding placeholder; production deployments should use a real embedding model).
3. It calls `MemoryManager.Search(chatCtx, query, 5)` to find the top 5 most similar memories.
4. If results are found, it formats them into a system message and prepends it to the message list:

```
Relevant context from previous conversations:
- User asked about deployment, I recommended Docker Compose.
- User had trouble with PostgreSQL connection pooling.
```

The LLM sees these memories as part of its context window, allowing it to reference past interactions.

### Persistence (OnMessagePersist)

1. The hook scans messages from the end to find the last assistant message.
2. It launches a goroutine to store the message asynchronously, so it doesn't block the response pipeline.
3. The memory is stored with metadata: session ID, role (`"assistant"`), and type (`"conversation"`).

If the store fails, the hook logs a warning and continues. A failed memory write never breaks the agent's response.

## Configuration

### YAML

```yaml
hooks:
  - name: "memory"
    type: "vector_store"
    enabled: true
    parameters:
      # Vector store backend
      vector_db: "qdrant"
      collection: "conversations"

      # Embedding model (for production use)
      embedding_model: "text-embedding-3-small"
      embedding_dimension: 1536

      # Retrieval settings
      top_k: 5
      min_similarity: 0.7

      # Retention
      retention_days: 90
      max_memories_per_session: 1000
```

### Go

```go
harness := core.NewHarness(core.HarnessConfig{
    Hooks: []HookSpec{
        {Name: "hooks.memory", Enabled: true},
    },
    Memory: core.MemoryConfig{
        Enabled:    true,
        TopK:       5,
        MinScore:   0.7,
    },
})
```

### Dependencies

The memory hook requires a `MemoryStore` implementation. CopCon provides a Qdrant-backed implementation. If `MemoryStore` is nil in the `CapabilityDeps`, the hook creates itself with a nil manager and becomes a no-op. No error, no crash, just silent skipping.

```go
func newMemoryManagerFromDeps(deps capabilities.CapabilityDeps) MemoryManager {
    if deps.MemoryStore == nil {
        return nil  // hook becomes a no-op
    }
    return &memoryManagerAdapter{store: deps.MemoryStore}
}
```

## MemoryManager Interface

The hook depends on this interface, which the `MemoryStore` storage interface satisfies:

```go
type MemoryManager interface {
    Store(chatCtx iface.ChatContextInterface, memory *storage.Memory) error
    Search(chatCtx iface.ChatContextInterface, query []float32, limit int) ([]*storage.Memory, error)
}
```

The `storage.Memory` value type:

```go
type Memory struct {
    ID         string
    Content    string
    AgentID    string
    Role       string
    Timestamp  time.Time
    MemoryType string
    Metadata   map[string]any
    Score      float32
}
```

## Performance Impact

| Aspect | Impact | Notes |
|--------|--------|-------|
| Retrieval latency | Vector search adds ~10-50ms per LLM call | Depends on vector store size and network |
| Persistence latency | Near-zero (async goroutine) | Does not block the response |
| Context window usage | Adds 1 system message with up to `top_k` results | Monitor token usage if memories are long |
| Memory overhead | One goroutine per persisted message | Goroutines are cheap, but high-throughput agents may accumulate many |

### Tips

- Set `top_k` conservatively (3-5) to avoid flooding the context window.
- Use `min_similarity` to filter out irrelevant memories.
- For production, replace the placeholder `encodeTextToVector` function with a real embedding model call. The current implementation is a naive byte-encoding meant for development only.

## Graceful Degradation

The memory hook is designed to work without a vector store:

- **No MemoryStore**: The hook is created with a nil manager. `Execute` returns immediately without doing anything.
- **Search fails**: The hook logs a warning and continues. The LLM call proceeds without memory context.
- **Store fails**: The hook logs a warning (from the async goroutine). The response is still sent to the user.

This means you can develop and test agents without Qdrant running, and enable memory later by providing a `MemoryStore`.

## Example: Chatbot with Memory

```yaml
agents:
  - name: "assistant"
    model: "gpt-4"
    system_prompt: |
      You are a helpful assistant. Use relevant context from
      previous conversations when it helps answer the user's question.
    hooks:
      - "memory"

hooks:
  - name: "memory"
    enabled: true
    parameters:
      top_k: 5
      min_similarity: 0.7
```

With this setup, the assistant will automatically pull in relevant past conversations and remember new ones.
