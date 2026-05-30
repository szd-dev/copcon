package knowledgebase

import (
	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/hook"
	kbtypes "github.com/copcon/plugins/knowledge-base/types"
)

type kbRecallHookCapabilityClosure struct {
	ks  KnowledgeStore
	emb kbtypes.Embedder
}

func (c *kbRecallHookCapabilityClosure) Name() string                      { return capabilities.HookKBRecall }
func (c *kbRecallHookCapabilityClosure) Type() capabilities.CapabilityType { return capabilities.CapabilityTypeHook }
func (c *kbRecallHookCapabilityClosure) DependsOn() []string               { return nil }

func (c *kbRecallHookCapabilityClosure) NewHook(deps capabilities.CapabilityDeps) (hook.Hook, error) {
	return NewKBRecallHook(c.emb, c.ks, deps.AgentKnowledgeBases), nil
}


