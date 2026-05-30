package knowledgebase

import (
	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/storage"
)

func RegisterCapabilities(r *capabilities.Registry, ks KnowledgeStore, emb storage.Embedder) {
	r.Register(&kbRecallHookCapabilityClosure{ks: ks, emb: emb})
}
