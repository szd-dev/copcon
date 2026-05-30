package hooks

import (
	"context"
	"fmt"

	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/providers/embedding"
	"github.com/copcon/core/storage"
)

type kbRecallHookCapability struct{}

func (c *kbRecallHookCapability) Name() string                      { return capabilities.HookKBRecall }
func (c *kbRecallHookCapability) Type() capabilities.CapabilityType { return capabilities.CapabilityTypeHook }
func (c *kbRecallHookCapability) DependsOn() []string               { return nil }

func (c *kbRecallHookCapability) NewHook(deps capabilities.CapabilityDeps) (hook.Hook, error) {
	if deps.KnowledgeStore == nil {
		return nil, fmt.Errorf("%w: KnowledgeStore not configured", capabilities.ErrDependencyUnavailable)
	}
	if deps.Embedder == nil {
		return nil, fmt.Errorf("%w: Embedder not configured", capabilities.ErrDependencyUnavailable)
	}

	type knowledgeStore interface {
		Search(ctx context.Context, kbIDs []string, query []float32, opts storage.SearchOptions) ([]*storage.Chunk, error)
	}

	ks, ok := deps.KnowledgeStore.(knowledgeStore)
	if !ok {
		return nil, fmt.Errorf("%w: KnowledgeStore type assertion failed", capabilities.ErrDependencyUnavailable)
	}

	emb, ok := deps.Embedder.(embedding.Embedder)
	if !ok {
		return nil, fmt.Errorf("%w: Embedder type assertion failed", capabilities.ErrDependencyUnavailable)
	}

	return NewKBRecallHook(emb, ks, deps.AgentKnowledgeBases), nil
}

func init() {
	capabilities.Register(&kbRecallHookCapability{})
}
