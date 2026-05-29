package hooks

import (
	"context"

	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/providers/embedding"
	"github.com/copcon/core/storage"
)

type kbRecallHookCapability struct{}

func (c *kbRecallHookCapability) Name() string                      { return "hooks.kb_recall" }
func (c *kbRecallHookCapability) Type() capabilities.CapabilityType { return capabilities.CapabilityTypeHook }
func (c *kbRecallHookCapability) DependsOn() []string               { return nil }

func (c *kbRecallHookCapability) NewHook(deps capabilities.CapabilityDeps) (hook.Hook, error) {
	if deps.KnowledgeStore == nil || deps.Embedder == nil {
		return nil, nil
	}

	type knowledgeStore interface {
		Search(ctx context.Context, kbIDs []string, query []float32, opts storage.SearchOptions) ([]*storage.Chunk, error)
	}

	ks, ok := deps.KnowledgeStore.(knowledgeStore)
	if !ok {
		return nil, nil
	}

	emb, ok := deps.Embedder.(embedding.Embedder)
	if !ok {
		return nil, nil
	}

	return NewKBRecallHook(emb, ks, deps.AgentKnowledgeBases), nil
}

func init() {
	capabilities.Register(&kbRecallHookCapability{})
}
