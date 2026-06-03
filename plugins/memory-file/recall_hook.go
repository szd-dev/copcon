package memoryfile

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/copcon/core/entity"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/llm"
)

// MemoryRecallHook selects relevant memories via LLM and injects them into
// the conversation context. Per-session dedup prevents double injection.
type MemoryRecallHook struct {
	store        *FileMemoryStore
	llm          llm.LLMProvider
	logger       *slog.Logger
	sessionCache sync.Map
}

func NewMemoryRecallHook(store *FileMemoryStore, llmProvider llm.LLMProvider) *MemoryRecallHook {
	return &MemoryRecallHook{
		store:  store,
		llm:    llmProvider,
		logger: slog.Default(),
	}
}

func (h *MemoryRecallHook) Name() string {
	return "memory_recall"
}

func (h *MemoryRecallHook) Points() []hook.HookPoint {
	return []hook.HookPoint{hook.AfterContextBuild}
}

func (h *MemoryRecallHook) Priority() int {
	return 70
}

// Execute reads the memory index, asks the LLM which entries are relevant,
// and injects selected memory content into the conversation.
func (h *MemoryRecallHook) Execute(ctx *hook.HookContext) error {
	if ctx.Messages == nil || len(*ctx.Messages) == 0 {
		return nil
	}

	agentID := ctx.AgentID
	if agentID == "" {
		return nil
	}

	if h.llm == nil {
		return nil
	}

	indexContent, err := ReadIndex(h.store.BasePath(), agentID)
	if err != nil {
		h.logger.Warn("failed to read memory index", "error", err)
		return nil
	}
	if strings.TrimSpace(indexContent) == "" {
		return nil
	}

	entries, err := collectEntries(AgentDir(h.store.BasePath(), agentID))
	if err != nil {
		h.logger.Warn("failed to collect index entries", "error", err)
		return nil
	}
	if len(entries) == 0 {
		return nil
	}

	// Find the user's last message.
	lastUserMsg := h.findLastUserMessage(*ctx.Messages)
	if lastUserMsg == "" {
		return nil
	}

	prompt := h.buildRecallPrompt(entries, lastUserMsg)

	response, err := Complete(ctx.ChatCtx.Context(), h.llm, llm.StreamParams{
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: prompt},
		},
		Temperature: 0.1,
		MaxTokens:   256,
	})
	if err != nil {
		h.logger.Warn("LLM recall request failed", "error", err)
		return nil
	}

	var selectedPaths []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(response)), &selectedPaths); err != nil {
		h.logger.Warn("failed to parse LLM recall response", "error", err, "response", response)
		return nil
	}
	if len(selectedPaths) == 0 {
		return nil
	}

	if len(selectedPaths) > 5 {
		selectedPaths = selectedPaths[:5]
	}

	sessionID := ctx.SessionID
	dedup := h.getOrCreateSessionCache(sessionID)
	var newPaths []string
	for _, p := range selectedPaths {
		if !dedup[p] {
			newPaths = append(newPaths, p)
		}
	}
	if len(newPaths) == 0 {
		return nil
	}

	var injections []entity.MessageForLLM
	for _, path := range newPaths {
		content, err := h.store.ReadFile(ctx.ChatCtx.Context(), agentID, path)
		if err != nil {
			h.logger.Warn("failed to read recalled memory file", "path", path, "error", err)
			continue
		}
		injections = append(injections, entity.MessageForLLM{
			Role:    "system",
			Content: fmt.Sprintf("[Recalled Memory: %s]\n%s", path, content),
		})
	}

	if len(injections) > 0 {
		msgs := append(injections, *ctx.Messages...)
		*ctx.Messages = msgs

		h.markInjected(sessionID, newPaths)
	}

	return nil
}

func (h *MemoryRecallHook) findLastUserMessage(msgs []entity.MessageForLLM) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			return msgs[i].Content
		}
	}
	return ""
}

func (h *MemoryRecallHook) buildRecallPrompt(entries []IndexEntry, userMessage string) string {
	var sb strings.Builder

	sb.WriteString("You are a memory recall assistant. Given the following memory index and the user's latest message, ")
	sb.WriteString("select the most relevant memories (up to 5) that would help the assistant respond.\n\n")

	sb.WriteString("Memory Index:\n")
	for i, e := range entries {
		desc := e.Description
		if desc == "" {
			desc = "(no description)"
		}
		sb.WriteString(fmt.Sprintf("%d. name: %q, path: %q, description: %q\n", i+1, e.Name, e.RelPath, desc))
	}

	sb.WriteString(fmt.Sprintf("\nUser's latest message: %q\n", userMessage))

	sb.WriteString("\nReturn a JSON array of memory paths. Example: [\"knowledge/prefs.md\"]\n")
	sb.WriteString("Return ONLY the JSON array, nothing else. If no memories are relevant, return [].\n")

	return sb.String()
}

func (h *MemoryRecallHook) getOrCreateSessionCache(sessionID string) map[string]bool {
	actual, _ := h.sessionCache.LoadOrStore(sessionID, &sync.Map{})
	cache, _ := actual.(*sync.Map)
	result := make(map[string]bool)
	cache.Range(func(key, _ any) bool {
		result[key.(string)] = true
		return true
	})
	return result
}

func (h *MemoryRecallHook) markInjected(sessionID string, paths []string) {
	actual, _ := h.sessionCache.LoadOrStore(sessionID, &sync.Map{})
	cache, _ := actual.(*sync.Map)
	for _, p := range paths {
		cache.Store(p, true)
	}
}
