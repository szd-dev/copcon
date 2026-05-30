package api

import (
	kbtypes "github.com/copcon/plugins/knowledge-base/types"
	knowledgebase "github.com/copcon/plugins/knowledge-base"
	kbrag "github.com/copcon/plugins/knowledge-base/rag"
)

func BuildKnowledgeOptions(ks knowledgebase.KnowledgeStore, emb kbtypes.Embedder) []HandlerOption {
	if ks == nil {
		return nil
	}

	opts := []HandlerOption{WithKnowledgeStore(ks)}

	if emb != nil {
		opts = append(opts, WithEmbedder(emb))

		pipeline := kbrag.NewPipeline(kbrag.NewDefaultParser(), emb, ks)
		opts = append(opts, WithRAGPipeline(pipeline))
	}

	return opts
}
