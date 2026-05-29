package storage

import (
	"fmt"
	"sync"
)

// KnowledgeStoreFactory creates a KnowledgeStore from a configuration map.
// Different providers extract their own config fields from the map.
type KnowledgeStoreFactory func(config map[string]any) (KnowledgeStore, error)

var (
	knowledgeStoreMu        sync.Mutex
	knowledgeStoreFactories = map[string]KnowledgeStoreFactory{}
)

// RegisterKnowledgeStoreProvider registers a KnowledgeStore backend factory.
// Panics if a provider with the same name has already been registered (fail-fast
// for programmer errors such as duplicate imports).
func RegisterKnowledgeStoreProvider(name string, factory KnowledgeStoreFactory) {
	knowledgeStoreMu.Lock()
	defer knowledgeStoreMu.Unlock()
	if _, exists := knowledgeStoreFactories[name]; exists {
		panic(fmt.Sprintf("storage: duplicate KnowledgeStore provider: %s", name))
	}
	knowledgeStoreFactories[name] = factory
}

// LookupKnowledgeStoreProvider returns the factory for the named backend.
// Returns an error if the backend is unknown.
func LookupKnowledgeStoreProvider(name string) (KnowledgeStoreFactory, error) {
	knowledgeStoreMu.Lock()
	defer knowledgeStoreMu.Unlock()
	f, ok := knowledgeStoreFactories[name]
	if !ok {
		return nil, fmt.Errorf("storage: unknown KnowledgeStore backend: %s (registered: %v)", name, providerNames())
	}
	return f, nil
}

// providerNames returns a sorted list of registered provider names for error messages.
func providerNames() []string {
	names := make([]string, 0, len(knowledgeStoreFactories))
	for n := range knowledgeStoreFactories {
		names = append(names, n)
	}
	return names
}