package knowledgebase

import (
	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/hook"
)

type kbRecallHookCapability struct{}

func (c *kbRecallHookCapability) Name() string                      { return capabilities.HookKBRecall }
func (c *kbRecallHookCapability) Type() capabilities.CapabilityType { return capabilities.CapabilityTypeHook }
func (c *kbRecallHookCapability) DependsOn() []string               { return nil }

func (c *kbRecallHookCapability) NewHook(deps capabilities.CapabilityDeps) (hook.Hook, error) {
	return nil, nil
}
