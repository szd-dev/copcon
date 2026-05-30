package capabilities

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMemoryBundleNamesNonEmpty(t *testing.T) {
	assert.NotEmpty(t, MemoryBundleNames())
}

func TestKnowledgeBaseBundleNamesNonEmpty(t *testing.T) {
	assert.NotEmpty(t, KnowledgeBaseBundleNames())
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

