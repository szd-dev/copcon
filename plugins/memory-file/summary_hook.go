package memoryfile

import (
	"context"
	"log/slog"
	"time"

	"github.com/copcon/core/hook"
)

// MemorySummaryHook triggers memory summarization on OnSystemPrompt,
// running after FileMemoryHook (priority 90 vs 80) so that system context
// files are already injected before any summary is triggered.
type MemorySummaryHook struct {
	summarizer *FileSummarizer
	logger     *slog.Logger
}

func NewMemorySummaryHook(summarizer *FileSummarizer) *MemorySummaryHook {
	return &MemorySummaryHook{
		summarizer: summarizer,
		logger:     slog.Default(),
	}
}

func (h *MemorySummaryHook) Name() string {
	return "memory_summary"
}

func (h *MemorySummaryHook) Points() []hook.HookPoint {
	return []hook.HookPoint{hook.OnSystemPrompt}
}

func (h *MemorySummaryHook) Priority() int {
	return 90
}

func (h *MemorySummaryHook) Execute(ctx *hook.HookContext) error {
	if h.summarizer == nil {
		return nil
	}

	agentID := ctx.AgentID
	if agentID == "" {
		return nil
	}

	if !h.summarizer.ShouldTrigger(agentID) {
		return nil
	}

	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		if err := h.summarizer.Summarize(bgCtx, agentID); err != nil {
			h.logger.Warn("memory summarization failed", "error", err)
		}
	}()

	return nil
}