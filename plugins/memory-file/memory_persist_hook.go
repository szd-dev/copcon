package memoryfile

import (
	"context"
	"log/slog"
	"math"
	"strings"
	"unicode"

	"github.com/copcon/core/hook"
	"github.com/copcon/core/storage"
)

type MemoryPersistHook struct {
	embedder    storage.Embedder
	memoryStore MemoryStore
	logger      *slog.Logger
}

func NewMemoryPersistHook(embedder storage.Embedder, memoryStore MemoryStore) *MemoryPersistHook {
	return &MemoryPersistHook{
		embedder:    embedder,
		memoryStore: memoryStore,
		logger:      slog.Default(),
	}
}

func (h *MemoryPersistHook) Name() string { return "memory_persist" }

func (h *MemoryPersistHook) Points() []hook.HookPoint {
	return []hook.HookPoint{hook.OnMessagePersist}
}

func (h *MemoryPersistHook) Priority() int { return 40 }

func (h *MemoryPersistHook) Execute(ctx *hook.HookContext) error {
	if h.embedder == nil || h.memoryStore == nil {
		return nil
	}

	if ctx.Messages == nil || len(*ctx.Messages) == 0 {
		return nil
	}

	for i := len(*ctx.Messages) - 1; i >= 0; i-- {
		msg := (*ctx.Messages)[i]
		if msg.Role == "assistant" && msg.Content != "" {
			content := msg.Content
			chatCtx := ctx.ChatCtx
			agentID := ctx.ChatCtx.AgentID()
			embedder := h.embedder
			store := h.memoryStore
			logger := h.logger

			go func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Warn("memory_persist: goroutine panic",
							"agent_id", agentID,
							"recover", r,
						)
					}
				}()

				h.persistAsync(chatCtx.Context(), agentID, content, embedder, store, logger)
			}()
			break
		}
	}

	return nil
}

func (h *MemoryPersistHook) persistAsync(
	ctx context.Context,
	agentID string,
	content string,
	embedder storage.Embedder,
	store MemoryStore,
	logger *slog.Logger,
) {
	keywords := extractKeywords(content, 5)
	if len(keywords) == 0 {
		return
	}

	vec, err := embedder.Embed(ctx, content)
	if err != nil {
		logger.Warn("memory_persist: embed failed",
			"agent_id", agentID,
			"error", err,
		)
		return
	}

	existing, err := store.Search(ctx, vec, 1)
	if err != nil {
		logger.Warn("memory_persist: similarity search failed",
			"agent_id", agentID,
			"error", err,
		)
	} else if len(existing) > 0 && cosineSim(vec, toFloat32Slice(existing[0].Metadata)) > 0.95 {
		return
	}

	memory := &storage.Memory{
		Content:    content,
		AgentID:    agentID,
		Role:       "assistant",
		MemoryType: string(storage.MemoryTypeSemantic),
		Metadata:   map[string]any{"keywords": strings.Join(keywords, ",")},
	}

	if err := store.Store(ctx, memory); err != nil {
		logger.Warn("memory_persist: store failed",
			"agent_id", agentID,
			"error", err,
		)
	}
}

func extractKeywords(text string, topN int) []string {
	words := tokenize(text)
	freq := make(map[string]int)
	for _, w := range words {
		if !isStopWord(w) {
			freq[w]++
		}
	}

	type kv struct {
		word string
		freq int
	}
	var sorted []kv
	for w, f := range freq {
		sorted = append(sorted, kv{w, f})
	}
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].freq > sorted[i].freq {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	n := topN
	if n > len(sorted) {
		n = len(sorted)
	}
	result := make([]string, n)
	for i := 0; i < n; i++ {
		result[i] = sorted[i].word
	}
	return result
}

func tokenize(text string) []string {
	text = strings.ToLower(text)
	var tokens []string
	var current strings.Builder

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

var englishStopWords = map[string]bool{
	"a": true, "an": true, "the": true, "and": true, "or": true, "but": true,
	"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
	"with": true, "by": true, "from": true, "is": true, "it": true, "that": true,
	"this": true, "was": true, "are": true, "be": true, "has": true, "had": true,
	"have": true, "will": true, "would": true, "could": true, "should": true,
	"not": true, "no": true, "do": true, "does": true, "did": true, "can": true,
	"i": true, "you": true, "he": true, "she": true, "we": true, "they": true,
	"me": true, "him": true, "her": true, "us": true, "them": true,
	"my": true, "your": true, "his": true, "its": true, "our": true, "their": true,
	"what": true, "which": true, "who": true, "when": true, "where": true, "how": true,
	"if": true, "then": true, "so": true, "than": true, "too": true, "very": true,
	"just": true, "about": true, "also": true, "up": true, "out": true,
}

var chineseStopWords = map[string]bool{
	"的": true, "了": true, "在": true, "是": true, "我": true, "有": true,
	"和": true, "就": true, "不": true, "人": true, "都": true, "一": true,
	"这": true, "中": true, "大": true, "为": true, "上": true, "个": true,
	"国": true, "他": true, "她": true, "它": true, "们": true, "那": true,
	"也": true, "要": true, "会": true, "对": true, "说": true, "与": true,
	"到": true, "很": true, "去": true, "能": true, "把": true, "被": true,
}

func isStopWord(word string) bool {
	if len(word) <= 1 {
		return true
	}
	if englishStopWords[word] {
		return true
	}
	if chineseStopWords[word] {
		return true
	}
	return false
}

func cosineSim(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}

func toFloat32Slice(m map[string]any) []float32 {
	if m == nil {
		return nil
	}
	vecAny, ok := m["vector"]
	if !ok {
		return nil
	}
	switch v := vecAny.(type) {
	case []float32:
		return v
	case []any:
		result := make([]float32, len(v))
		for i, val := range v {
			if f, ok := val.(float32); ok {
				result[i] = f
			} else if f, ok := val.(float64); ok {
				result[i] = float32(f)
			}
		}
		return result
	}
	return nil
}


