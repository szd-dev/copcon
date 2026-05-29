package hooks

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/copcon/core/entity"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/providers/embedding"
	"github.com/copcon/core/storage"
)

type KBRecallHook struct {
	embedder embedding.Embedder
	kbStore  KnowledgeStoreReader
	kbIDs    []string
	logger   *slog.Logger
}

type KnowledgeStoreReader interface {
	Search(ctx context.Context, kbIDs []string, query []float32, opts storage.SearchOptions) ([]*storage.Chunk, error)
}

func NewKBRecallHook(embedder embedding.Embedder, kbStore KnowledgeStoreReader, kbIDs []string) *KBRecallHook {
	return &KBRecallHook{
		embedder: embedder,
		kbStore:  kbStore,
		kbIDs:    kbIDs,
		logger:   slog.Default(),
	}
}

func (h *KBRecallHook) Name() string { return "kb_recall" }

func (h *KBRecallHook) Points() []hook.HookPoint {
	return []hook.HookPoint{hook.AfterContextBuild}
}

func (h *KBRecallHook) Priority() int { return 60 }

func (h *KBRecallHook) Execute(ctx *hook.HookContext) error {
	if h.embedder == nil || h.kbStore == nil || len(h.kbIDs) == 0 {
		return nil
	}

	if ctx.Messages == nil || len(*ctx.Messages) == 0 {
		return nil
	}

	query := h.findLastUserMessage(*ctx.Messages)
	if query == "" {
		return nil
	}

	queryVec, err := h.embedder.Embed(ctx.ChatCtx.Context(), query)
	if err != nil {
		h.logger.Warn("kb_recall: embed query failed",
			"session_id", ctx.SessionID,
			"error", err,
		)
		return nil
	}

	results, err := h.kbStore.Search(ctx.ChatCtx.Context(), h.kbIDs, queryVec, storage.SearchOptions{
		TopK:                5,
		SimilarityThreshold: 0.5,
	})
	if err != nil {
		h.logger.Warn("kb_recall: search failed",
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
		Content: formatKBResults(results),
	}

	*ctx.Messages = append([]entity.MessageForLLM{systemMsg}, *ctx.Messages...)
	return nil
}

func (h *KBRecallHook) findLastUserMessage(messages []entity.MessageForLLM) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" && messages[i].Content != "" {
			return messages[i].Content
		}
	}
	return ""
}

func formatKBResults(results []*storage.Chunk) string {
	out := "Relevant knowledge base content:\n"
	for i, r := range results {
		out += fmt.Sprintf("\n[%d] (score: %.3f) %s", i+1, r.Score, r.Content)
	}
	return out
}
