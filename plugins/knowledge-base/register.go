package knowledgebase

import "github.com/copcon/core/capabilities"

func RegisterCapabilities(r *capabilities.Registry) {
	r.Register(&kbRecallHookCapability{})
	r.Register(&memoryPersistHookCapability{})
}
