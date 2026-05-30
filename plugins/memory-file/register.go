package memoryfile

import "github.com/copcon/core/capabilities"

func RegisterCapabilities(r *capabilities.Registry, store *FileMemoryStore) {
	r.Register(&MemoryModule{store: store})
}