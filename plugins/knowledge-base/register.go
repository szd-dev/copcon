package knowledgebase

import (
	"github.com/copcon/core/capabilities"
	kbtypes "github.com/copcon/plugins/knowledge-base/types"
)

func RegisterCapabilities(r *capabilities.Registry, ks KnowledgeStore, emb kbtypes.Embedder) {
	r.Register(&kbRecallHookCapabilityClosure{ks: ks, emb: emb})
}
