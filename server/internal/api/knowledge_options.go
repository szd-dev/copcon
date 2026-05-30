package api

import (
	"github.com/copcon/core/storage"
	"github.com/copcon/plugins/knowledge-base"
	"github.com/copcon/plugins/rag"
)

func BuildKnowledgeOptions(ks knowledgebase.KnowledgeStore, emb storage.Embedder) []HandlerOption {
	if ks == nil {
		return nil
	}

	opts := []HandlerOption{WithKnowledgeStore(ks)}

	if emb != nil {
		opts = append(opts, WithEmbedder(emb))

		pipeline := rag.NewPipeline(rag.NewDefaultParser(), emb, ks)
		opts = append(opts, WithRAGPipeline(pipeline))
	}

	return opts
}
