package sqlitevec

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	knowledgebase "github.com/copcon/plugins/knowledge-base"
	kbtypes "github.com/copcon/plugins/knowledge-base/types"
)

// verifyMock is a VectorStore mock that returns configurable counts from Verify.
type verifyMock struct {
	counts map[string]int
	err    error
}

func (m *verifyMock) Store(ctx context.Context, kbID, docID string, chunks []knowledgebase.VectorChunk, vectors [][]float32) error {
	return nil
}
func (m *verifyMock) Search(ctx context.Context, kbIDs []string, query []float32, opts kbtypes.SearchOptions) ([]knowledgebase.SearchResult, error) {
	return nil, nil
}
func (m *verifyMock) DeleteByKB(ctx context.Context, kbID string) error                { return nil }
func (m *verifyMock) DeleteByDocument(ctx context.Context, kbID, docID string) error     { return nil }
func (m *verifyMock) Backend() string                                                    { return "verify-mock" }
func (m *verifyMock) Verify(ctx context.Context) (map[string]int, error)                 { return m.counts, m.err }

var _ knowledgebase.VectorStore = (*verifyMock)(nil)

func TestConsistencyCheck_MarksUnavailable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Use no-op mock for initial creation (no consistency check issues yet)
	ks, err := NewKnowledgeStore(db, &mockVectorStore{})
	require.NoError(t, err)

	ctx := context.Background()

	// Create a KB with a document that claims 5 chunks
	kb, err := ks.CreateKB(ctx, &kbtypes.KnowledgeBase{Name: "test-kb", Backend: "mock"})
	require.NoError(t, err)

	doc := &kbtypes.Document{
		KBID: kb.ID, Filename: "doc.txt", Source: "upload",
		Status: kbtypes.DocStatusReady, ChunkCount: 5,
	}
	err = ks.IngestDocument(ctx, kb.ID, doc, nil)
	require.NoError(t, err)

	// Now create a NEW KnowledgeStore with a verifyMock that reports only 3 chunks
	// This simulates inconsistency: metadata says 5 chunks, vector store has 3
	vm := &verifyMock{counts: map[string]int{kb.ID: 3}}
	ks2, err := NewKnowledgeStore(db, vm)
	require.NoError(t, err)

	// Fetch the KB and check it's marked unavailable
	got, err := ks2.GetKB(ctx, kb.ID)
	require.NoError(t, err)
	assert.Equal(t, false, got.Config["available"])
	assert.Contains(t, got.Config["unavailable_reason"], "mismatch")
	_ = ks // suppress unused warning
}

func TestConsistencyCheck_MarksAvailable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	ks, err := NewKnowledgeStore(db, &mockVectorStore{})
	require.NoError(t, err)

	ctx := context.Background()

	// Create KB with a document claiming 3 chunks
	kb, err := ks.CreateKB(ctx, &kbtypes.KnowledgeBase{Name: "ok-kb", Backend: "mock"})
	require.NoError(t, err)

	doc := &kbtypes.Document{
		KBID: kb.ID, Filename: "doc.txt", Source: "upload",
		Status: kbtypes.DocStatusReady, ChunkCount: 3,
	}
	err = ks.IngestDocument(ctx, kb.ID, doc, nil)
	require.NoError(t, err)

	// verifyMock reports exactly 3 chunks — consistent
	vm := &verifyMock{counts: map[string]int{kb.ID: 3}}
	ks2, err := NewKnowledgeStore(db, vm)
	require.NoError(t, err)

	got, err := ks2.GetKB(ctx, kb.ID)
	require.NoError(t, err)
	assert.Equal(t, true, got.Config["available"])
}

func TestConsistencyCheck_EmptyKB(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// verifyMock returns empty map — no KBs in vector store
	vm := &verifyMock{counts: map[string]int{}}
	ks, err := NewKnowledgeStore(db, vm)
	require.NoError(t, err)

	ctx := context.Background()

	// Create KB with no documents
	kb, err := ks.CreateKB(ctx, &kbtypes.KnowledgeBase{Name: "empty-kb", Backend: "mock"})
	require.NoError(t, err)

	// Reconstruct to trigger consistency check (0 expected, 0 actual → consistent)
	vm2 := &verifyMock{counts: map[string]int{}}
	ks2, err := NewKnowledgeStore(db, vm2)
	require.NoError(t, err)

	got, err := ks2.GetKB(ctx, kb.ID)
	require.NoError(t, err)
	assert.Equal(t, true, got.Config["available"])
}
