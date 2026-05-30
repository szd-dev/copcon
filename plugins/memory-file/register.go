package memoryfile

import "github.com/copcon/core/capabilities"

func RegisterCapabilities(r *capabilities.Registry) {
	r.Register(&fileMemoryHookCapability{})
	r.Register(&memoryHookCapability{})
	r.Register(&memoryStoreCapability{})
	r.Register(&memoryRecallCapability{})
	r.Register(&memoryForgetType{})
}
