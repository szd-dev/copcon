package storage

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegisterAndLookup(t *testing.T) {
	// Use a unique name so this test can run in parallel with others.
	name := t.Name()
	factory := KnowledgeStoreFactory(func(cfg map[string]any) (KnowledgeStore, error) {
		return nil, nil
	})

	RegisterKnowledgeStoreProvider(name, factory)

	got, err := LookupKnowledgeStoreProvider(name)
	require.NoError(t, err)
	require.NotNil(t, got)
}

func TestLookupUnknown(t *testing.T) {
	_, err := LookupKnowledgeStoreProvider("nonexistent-backend")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown KnowledgeStore backend")
}

func TestDuplicateRegisterPanics(t *testing.T) {
	name := t.Name()
	factory := KnowledgeStoreFactory(func(cfg map[string]any) (KnowledgeStore, error) {
		return nil, nil
	})

	RegisterKnowledgeStoreProvider(name, factory)

	require.Panics(t, func() {
		RegisterKnowledgeStoreProvider(name, factory)
	}, "expected panic on duplicate registration")
}

func TestConcurrentAccess(t *testing.T) {
	done := make(chan struct{})

	// Register 10 different backends concurrently.
	for i := 0; i < 10; i++ {
		i := i
		go func() {
			name := t.Name() + "-register-" + string(rune('0'+i))
			factory := KnowledgeStoreFactory(func(cfg map[string]any) (KnowledgeStore, error) {
				return nil, nil
			})
			RegisterKnowledgeStoreProvider(name, factory)
			done <- struct{}{}
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}

	// Lookup 10 different backends concurrently.
	for i := 0; i < 10; i++ {
		i := i
		go func() {
			name := t.Name() + "-register-" + string(rune('0'+i))
			_, err := LookupKnowledgeStoreProvider(name)
			require.NoError(t, err)
			done <- struct{}{}
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}