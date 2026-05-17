// Package memory provides the MemoryPlugin, a hook that injects relevant
// memories into the LLM context and stores new assistant messages for
// future retrieval.
package memory

import (
	"fmt"
	"log/slog"

	"github.com/copcon/server/internal/domain/entity"
	"github.com/copcon/server/internal/hook"
	"github.com/copcon/server/internal/memory"
)

// MemoryPlugin implements hook.Hook to inject vector-search results into
// the context window (AfterContextBuild) and persist assistant messages
// to the memory store (OnMessagePersist).
type MemoryPlugin struct {
	memoryMgr memory.MemoryManager
	logger    *slog.Logger
}

// NewMemoryPlugin creates a new MemoryPlugin. If memoryMgr is nil, all
// hook operations become no-ops (graceful degradation).
func NewMemoryPlugin(memoryMgr memory.MemoryManager) *MemoryPlugin {
	return &MemoryPlugin{
		memoryMgr: memoryMgr,
		logger:    slog.Default(),
	}
}

// Name returns a human-readable identifier for logging and debugging.
func (p *MemoryPlugin) Name() string {
	return "memory_plugin"
}

// Points returns the hook points at which this hook should execute.
func (p *MemoryPlugin) Points() []hook.HookPoint {
	return []hook.HookPoint{hook.AfterContextBuild, hook.OnMessagePersist}
}

// Priority returns the execution order. 100 is the default priority.
func (p *MemoryPlugin) Priority() int {
	return 100
}

// Execute dispatches to the appropriate handler based on the current
// hook point. Errors are logged but never returned — the hook never
// aborts the pipeline.
func (p *MemoryPlugin) Execute(ctx *hook.HookContext) error {
	if p.memoryMgr == nil {
		return nil
	}

	switch ctx.CurrentPoint {
	case hook.AfterContextBuild:
		return p.onAfterContextBuild(ctx)
	case hook.OnMessagePersist:
		return p.onMessagePersist(ctx)
	default:
		return nil
	}
}

// onAfterContextBuild searches the memory store using the last user
// message as a query and prepends relevant memories as a system message
// to the context window.
func (p *MemoryPlugin) onAfterContextBuild(ctx *hook.HookContext) error {
	if ctx.Messages == nil || len(*ctx.Messages) == 0 {
		return nil
	}

	// Find the last user message to use as the search query.
	lastUserContent := p.findLastUserMessage(*ctx.Messages)
	if lastUserContent == "" {
		return nil
	}

	// Build a query vector from the user message text. This is a
	// simple placeholder encoding — the real MemoryManager
	// implementation is responsible for proper embedding generation.
	query := encodeTextToVector(lastUserContent)

	results, err := p.memoryMgr.Search(ctx.ChatCtx, query, 5)
	if err != nil {
		p.logger.Warn("memory search failed",
			"session_id", ctx.SessionID,
			"error", err,
		)
		return nil
	}

	if len(results) == 0 {
		return nil
	}

	// Format results as a system message and prepend to the message list.
	systemMsg := entity.MessageForLLM{
		Role:    "system",
		Content: formatSearchResults(results),
	}

	*ctx.Messages = append([]entity.MessageForLLM{systemMsg}, *ctx.Messages...)

	return nil
}

// onMessagePersist stores assistant messages with non-empty content
// to the memory store. Storage happens asynchronously via a goroutine
// to avoid blocking the agent loop.
func (p *MemoryPlugin) onMessagePersist(ctx *hook.HookContext) error {
	if ctx.Messages == nil || len(*ctx.Messages) == 0 {
		return nil
	}

	// Only store the last assistant message with content.
	for i := len(*ctx.Messages) - 1; i >= 0; i-- {
		msg := (*ctx.Messages)[i]
		if msg.Role == "assistant" && msg.Content != "" {
			// Capture values for the goroutine.
			chatCtx := ctx.ChatCtx
			sessionID := ctx.SessionID
			content := msg.Content
			mgr := p.memoryMgr
			logger := p.logger

			go func() {
				err := mgr.Store(chatCtx, &memory.Memory{
					Content:    content,
					SessionID:  sessionID,
					Role:       "assistant",
					MemoryType: "conversation",
				})
				if err != nil {
					logger.Warn("memory store failed",
						"session_id", sessionID,
						"error", err,
					)
				}
			}()
			break
		}
	}

	return nil
}

// findLastUserMessage scans messages in reverse for the most recent user
// message with content and returns its content.
func (p *MemoryPlugin) findLastUserMessage(messages []entity.MessageForLLM) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" && messages[i].Content != "" {
			return messages[i].Content
		}
	}
	return ""
}

// encodeTextToVector converts text to a simple float32 vector for
// the memory search query. This is a placeholder — production use
// should replace this with a proper embedding model.
func encodeTextToVector(text string) []float32 {
	bytes := []byte(text)
	vec := make([]float32, len(bytes))
	for i, b := range bytes {
		vec[i] = float32(b) / 255.0
	}
	return vec
}

// formatSearchResults formats memory search results into a system
// message string listing relevant context.
func formatSearchResults(results []*memory.Memory) string {
	out := "Relevant context from previous conversations:\n"
	for _, r := range results {
		out += fmt.Sprintf("- %s\n", r.Content)
	}
	return out
}
