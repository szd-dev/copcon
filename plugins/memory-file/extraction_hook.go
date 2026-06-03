package memoryfile

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/copcon/core/hook"
	"github.com/copcon/core/llm"
	"github.com/copcon/core/storage"
	"github.com/google/uuid"
)

// manualStoreFlags tracks sessions where the agent manually stored memories.
// When set, FactExtractionHook skips extraction to prevent duplicates.
var manualStoreFlags sync.Map // sessionID → bool

// SetManualStoreFlag marks a session as having manual memory storage.
func SetManualStoreFlag(sessionID string) {
	manualStoreFlags.Store(sessionID, true)
}

// ClearManualStoreFlag clears the flag for a session.
func ClearManualStoreFlag(sessionID string) {
	manualStoreFlags.Delete(sessionID)
}

// extractedFact represents a single fact extracted by the LLM.
type extractedFact struct {
	Content     string  `json:"content"`
	Type        string  `json:"type"`        // user/feedback/project/reference
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Importance  float64 `json:"importance"`
}

// FactExtractionHook asynchronously extracts facts from conversation messages
// using an LLM and writes them as memory files.
type FactExtractionHook struct {
	store        *FileMemoryStore
	llm          llm.LLMProvider
	messageStore storage.MessageStore
	logger       *slog.Logger
	model        string
}

// NewFactExtractionHook creates a new FactExtractionHook.
func NewFactExtractionHook(store *FileMemoryStore, llmProvider llm.LLMProvider, msgStore storage.MessageStore, model string) *FactExtractionHook {
	return &FactExtractionHook{
		store:        store,
		llm:          llmProvider,
		messageStore: msgStore,
		logger:       slog.Default(),
		model:        model,
	}
}

func (h *FactExtractionHook) Name() string {
	return "fact_extraction"
}

func (h *FactExtractionHook) Points() []hook.HookPoint {
	return []hook.HookPoint{hook.OnMessagePersist}
}

func (h *FactExtractionHook) Priority() int {
	return 100
}

func (h *FactExtractionHook) Execute(ctx *hook.HookContext) error {
	// 1. Check manual store flag — skip if agent already stored manually.
	sessionID := ctx.SessionID
	if _, loaded := manualStoreFlags.Load(sessionID); loaded {
		return nil
	}

	// 2. Check LLM available.
	if h.llm == nil {
		return nil
	}

	// 3. Capture values for the goroutine.
	agentID := ctx.AgentID
	store := h.store
	llmProvider := h.llm
	msgStore := h.messageStore
	logger := h.logger
	model := h.model

	// 4. Launch async goroutine — NEVER block the pipeline.
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		sessionUUID, err := uuid.Parse(sessionID)
		if err != nil {
			logger.Warn("invalid session ID for fact extraction", "sessionID", sessionID)
			return
		}

		messages, err := msgStore.List(bgCtx, sessionUUID, 3)
		if err != nil || len(messages) == 0 {
			return
		}

		var convoBuilder strings.Builder
		var messageIDs []string
		for _, msg := range messages {
			role := msg.Role
			content := msg.Content
			if content == "" {
				continue
			}
			convoBuilder.WriteString(fmt.Sprintf("%s: %q\n\n", strings.Title(role), content))
			messageIDs = append(messageIDs, msg.ID.String())
		}
		conversationText := convoBuilder.String()
		if conversationText == "" {
			return
		}

		existingMemories := h.buildExistingMemoryList(bgCtx, agentID, store)

		prompt := buildExtractionPrompt(existingMemories, conversationText)

		params := llm.StreamParams{
			Model: model,
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: prompt},
			},
			Temperature: 0.1,
		}

		result, err := Complete(bgCtx, llmProvider, params)
		if err != nil {
			logger.Warn("fact extraction LLM call failed", "error", err)
			return
		}

		result = strings.TrimSpace(result)
		if strings.HasPrefix(result, "```") {
			if idx := strings.Index(result[3:], "\n"); idx >= 0 {
				result = result[3+idx+1:]
			}
			if idx := strings.LastIndex(result, "```"); idx >= 0 {
				result = result[:idx]
			}
			result = strings.TrimSpace(result)
		}

		var facts []extractedFact
		if err := json.Unmarshal([]byte(result), &facts); err != nil {
			logger.Warn("fact extraction JSON parse failed", "error", err, "raw", truncate(result, 200))
			return
		}

		if len(facts) == 0 {
			return
		}

		if len(facts) > 5 {
			facts = facts[:5]
		}

		for _, fact := range facts {
			if fact.Content == "" || fact.Name == "" {
				continue
			}

			now := time.Now()
			relPath := filepath.Join("knowledge", fact.Name+".md")

			fm := Frontmatter{
				Name:        fact.Name,
				Category:    "knowledge",
				Description: fact.Description,
				Type:        fact.Type,
				Importance:  fact.Importance,
				SessionID:   sessionID,
				MessageIDs:  messageIDs,
				CreatedAt:   now,
				UpdatedAt:   now,
			}

			data := SerializeFrontmatter(fm, fact.Content)
			fullPath := filepath.Join(store.BasePath(), agentID, relPath)

			if err := WriteFileWithPerms(fullPath, data); err != nil {
				logger.Warn("failed to write extracted fact", "path", relPath, "error", err)
				continue
			}
		}

		store.mu.Lock()
		defer store.mu.Unlock()
		if err := BuildIndex(store.BasePath(), agentID, store.maxIndexLines, store.maxIndexBytes); err != nil {
			logger.Warn("failed to rebuild index after fact extraction", "error", err)
		}
		if err := BuildFacts(store.BasePath(), agentID); err != nil {
			logger.Warn("failed to rebuild facts after fact extraction", "error", err)
		}
	}()

	return nil
}

func (h *FactExtractionHook) buildExistingMemoryList(ctx context.Context, agentID string, store *FileMemoryStore) string {
	memories, err := store.GetByAgentID(ctx, agentID, 50)
	if err != nil || len(memories) == 0 {
		return ""
	}

	var lines []string
	for _, m := range memories {
		name := m.ID
		content := m.Content
		if len(content) > 80 {
			content = content[:80] + "..."
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", name, content))
	}
	return strings.Join(lines, "\n")
}

func buildExtractionPrompt(existingMemories, conversation string) string {
	var sb strings.Builder

	sb.WriteString("You are a fact extraction assistant. Analyze the following conversation and extract important facts worth remembering.\n\n")

	if existingMemories != "" {
		sb.WriteString("Existing memories (do NOT duplicate these):\n")
		sb.WriteString(existingMemories)
		sb.WriteString("\n\n")
	}

	sb.WriteString("Recent conversation:\n")
	sb.WriteString(conversation)
	sb.WriteString("\n")

	sb.WriteString(`Extract up to 5 new facts. For each fact provide:
- content: The factual content
- type: One of "user" (preferences), "feedback" (corrections), "project" (project info), "reference" (reference data)
- name: Short identifier (no spaces, use underscores)
- description: Brief one-line description
- importance: Score 0.0-1.0

Return JSON array: [{"content":"...","type":"...","name":"...","description":"...","importance":0.8}]
If no new facts worth extracting, return [].
Return ONLY the JSON array.`)

	return sb.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
