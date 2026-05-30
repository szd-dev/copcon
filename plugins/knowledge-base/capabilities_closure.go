package knowledgebase

import (
	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/storage"
)

type kbRecallHookCapabilityClosure struct {
	ks  KnowledgeStore
	emb storage.Embedder
}

func (c *kbRecallHookCapabilityClosure) Name() string                      { return capabilities.HookKBRecall }
func (c *kbRecallHookCapabilityClosure) Type() capabilities.CapabilityType { return capabilities.CapabilityTypeHook }
func (c *kbRecallHookCapabilityClosure) DependsOn() []string               { return nil }

func (c *kbRecallHookCapabilityClosure) NewHook(deps capabilities.CapabilityDeps) (hook.Hook, error) {
	return NewKBRecallHook(c.emb, c.ks, deps.AgentKnowledgeBases), nil
}


