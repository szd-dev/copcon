package hooks

import (
	"fmt"
	"log/slog"

	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/entity"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/iface"
	"github.com/copcon/core/storage"
)

type MemoryPlugin struct {
	memoryMgr MemoryManager
	logger    *slog.Logger
}

func NewMemoryPlugin(memoryMgr MemoryManager) *MemoryPlugin {
	return &MemoryPlugin{
		memoryMgr: memoryMgr,
		logger:    slog.Default(),
	}
}

func (p *MemoryPlugin) Name() string {
	return "memory_plugin"
}

func (p *MemoryPlugin) Points() []hook.HookPoint {
	return []hook.HookPoint{hook.AfterContextBuild, hook.OnMessagePersist}
}

func (p *MemoryPlugin) Priority() int {
	return 100
}

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

func (p *MemoryPlugin) onAfterContextBuild(ctx *hook.HookContext) error {
	if ctx.Messages == nil || len(*ctx.Messages) == 0 {
		return nil
	}

	lastUserContent := p.findLastUserMessage(*ctx.Messages)
	if lastUserContent == "" {
		return nil
	}

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

	systemMsg := entity.MessageForLLM{
		Role:    "system",
		Content: formatSearchResults(results),
	}

	*ctx.Messages = append([]entity.MessageForLLM{systemMsg}, *ctx.Messages...)

	return nil
}

func (p *MemoryPlugin) onMessagePersist(ctx *hook.HookContext) error {
	if ctx.Messages == nil || len(*ctx.Messages) == 0 {
		return nil
	}

	for i := len(*ctx.Messages) - 1; i >= 0; i-- {
		msg := (*ctx.Messages)[i]
		if msg.Role == "assistant" && msg.Content != "" {
			chatCtx := ctx.ChatCtx
			sessionID := ctx.SessionID
			content := msg.Content
			mgr := p.memoryMgr
			logger := p.logger

			go func() {
				err := mgr.Store(chatCtx, &storage.Memory{
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

func (p *MemoryPlugin) findLastUserMessage(messages []entity.MessageForLLM) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" && messages[i].Content != "" {
			return messages[i].Content
		}
	}
	return ""
}

func encodeTextToVector(text string) []float32 {
	bytes := []byte(text)
	vec := make([]float32, len(bytes))
	for i, b := range bytes {
		vec[i] = float32(b) / 255.0
	}
	return vec
}

func formatSearchResults(results []*storage.Memory) string {
	out := "Relevant context from previous conversations:\n"
	for _, r := range results {
		out += fmt.Sprintf("- %s\n", r.Content)
	}
	return out
}

func init() {
	capabilities.Register(&memoryHookCapability{})
}

type memoryHookCapability struct{}

func (c *memoryHookCapability) Name() string                         { return capabilities.HookMemory }
func (c *memoryHookCapability) Type() capabilities.CapabilityType    { return capabilities.CapabilityTypeHook }
func (c *memoryHookCapability) DependsOn() []string                  { return nil }
func (c *memoryHookCapability) NewHook(deps capabilities.CapabilityDeps) (hook.Hook, error) {
	if deps.MemoryStore == nil {
		return nil, fmt.Errorf("%w: MemoryStore not configured", capabilities.ErrDependencyUnavailable)
	}
	return NewMemoryPlugin(newMemoryManagerFromDeps(deps)), nil
}

func newMemoryManagerFromDeps(deps capabilities.CapabilityDeps) MemoryManager {
	if deps.MemoryStore == nil {
		return nil
	}
	return &memoryManagerAdapter{store: deps.MemoryStore}
}

type memoryManagerAdapter struct {
	store storage.MemoryStore
}

func (a *memoryManagerAdapter) Store(chatCtx iface.ChatContextInterface, memory *storage.Memory) error {
	return a.store.Store(chatCtx.Context(), memory)
}

func (a *memoryManagerAdapter) Search(chatCtx iface.ChatContextInterface, query []float32, limit int) ([]*storage.Memory, error) {
	return a.store.Search(chatCtx.Context(), query, limit)
}