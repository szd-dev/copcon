package capabilities

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMemoryBundleNamesNonEmpty(t *testing.T) {
	names := MemoryBundleNames()
	assert.NotEmpty(t, names, "MemoryBundleNames should return at least one name")
}

func TestKnowledgeBaseBundleNamesNonEmpty(t *testing.T) {
	names := KnowledgeBaseBundleNames()
	assert.NotEmpty(t, names, "KnowledgeBaseBundleNames should return at least one name")
}

func TestBundlesAreDisjoint(t *testing.T) {
	mem := make(map[string]bool)
	for _, n := range MemoryBundleNames() {
		mem[n] = true
	}
	for _, n := range KnowledgeBaseBundleNames() {
		if mem[n] {
			t.Errorf("name %q appears in both MemoryBundleNames and KnowledgeBaseBundleNames", n)
		}
	}
}

func TestMemoryBundleNamesCount(t *testing.T) {
	names := MemoryBundleNames()
	assert.Len(t, names, 4, "MemoryBundleNames should have exactly 4 entries")
}

func TestKnowledgeBaseBundleNamesCount(t *testing.T) {
	names := KnowledgeBaseBundleNames()
	assert.Len(t, names, 2, "KnowledgeBaseBundleNames should have exactly 2 entries")
}