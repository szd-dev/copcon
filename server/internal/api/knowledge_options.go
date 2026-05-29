package api

import (
	"log/slog"
	"os"

	"github.com/copcon/core/llm"
	"github.com/copcon/core/providers/embedding"
	"github.com/copcon/core/rag"
	"github.com/copcon/core/storage"
	"github.com/copcon/server/internal/config"
)

func BuildKnowledgeOptions(cfg *config.Config, storeProvider storage.StoreProvider, llmProvider llm.LLMProvider) []HandlerOption {
	var opts []HandlerOption

	ks := storeProvider.Knowledge()
	if ks == nil {
		return opts
	}

	embCfg := resolveEmbeddingConfig(cfg)
	var emb embedding.Embedder
	if embCfg.Backend != "" {
		var err error
		emb, err = embedding.NewFromConfig(embCfg, llmProvider)
		if err != nil {
			slog.New(slog.NewTextHandler(os.Stderr, nil)).Warn("failed to create embedder for API", "error", err)
		}
	}

	if emb != nil {
		opts = append(opts, WithEmbedder(emb))

		if ps, ok := ks.(rag.PipelineStore); ok {
			pipeline := rag.NewPipeline(rag.NewDefaultParser(), emb, ps)
			opts = append(opts, WithRAGPipeline(pipeline))
		}
	}

	return opts
}

func resolveEmbeddingConfig(cfg *config.Config) embedding.EmbeddingConfig {
	for _, kb := range cfg.KnowledgeBases {
		if kb.Embedding.Backend != "" {
			return embedding.EmbeddingConfig{
				Backend:     embedding.BackendType(kb.Embedding.Backend),
				BaseURL:     cfg.OpenAI.BaseURL,
				APIKey:      cfg.OpenAI.APIKey,
				OpenAIModel: kb.Embedding.OpenAIModel,
			}
		}
	}
	return embedding.EmbeddingConfig{}
}
