package memoryfile

import "github.com/copcon/core/capabilities"

func RegisterCapabilities(r *capabilities.Registry, store *FileMemoryStore) {
	r.Register(&fileMemoryHookCapabilityClosure{store: store})
	r.Register(&memoryHookCapabilityClosure{store: store})
	r.Register(&memoryStoreCapabilityClosure{store: store})
	r.Register(&memoryRecallCapabilityClosure{store: store})
	r.Register(&memoryForgetCapabilityClosure{store: store})
}
