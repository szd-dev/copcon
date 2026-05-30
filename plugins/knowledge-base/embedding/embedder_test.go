package kbembedding

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockEmbedder implements Embedder for testing.
type mockEmbedder struct {
	embedFunc      func(ctx context.Context, text string) ([]float32, error)
	embedBatchFunc func(ctx context.Context, texts []string) ([][]float32, error)
	dimensions     int
	name           string
}

var _ Embedder = (*mockEmbedder)(nil)

func (m *mockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return m.embedFunc(ctx, text)
}

func (m *mockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	return m.embedBatchFunc(ctx, texts)
}

func (m *mockEmbedder) Dimensions() int {
	return m.dimensions
}

func (m *mockEmbedder) Name() string {
	return m.name
}

func TestMockEmbedder(t *testing.T) {
	ctx := context.Background()

	m := &mockEmbedder{
		embedFunc: func(_ context.Context, text string) ([]float32, error) {
			return []float32{0.1, 0.2, 0.3}, nil
		},
		embedBatchFunc: func(_ context.Context, texts []string) ([][]float32, error) {
			vectors := make([][]float32, len(texts))
			for i := range texts {
				vectors[i] = []float32{float32(i), 0.2, 0.3}
			}
			return vectors, nil
		},
		dimensions: 3,
		name:       "test:mock",
	}

	t.Run("Embed", func(t *testing.T) {
		vec, err := m.Embed(ctx, "hello")
		assert.NoError(t, err)
		assert.Equal(t, []float32{0.1, 0.2, 0.3}, vec)
	})

	t.Run("EmbedBatch", func(t *testing.T) {
		vecs, err := m.EmbedBatch(ctx, []string{"a", "b", "c"})
		assert.NoError(t, err)
		assert.Len(t, vecs, 3)
		assert.Equal(t, []float32{0, 0.2, 0.3}, vecs[0])
		assert.Equal(t, []float32{1, 0.2, 0.3}, vecs[1])
		assert.Equal(t, []float32{2, 0.2, 0.3}, vecs[2])
	})

	t.Run("Dimensions", func(t *testing.T) {
		assert.Equal(t, 3, m.Dimensions())
	})

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "test:mock", m.Name())
	})

	t.Run("ImplementsEmbedder", func(t *testing.T) {
		// Compile-time check is at the top of the file (var _ Embedder = (*mockEmbedder)(nil)).
		// Runtime check: verify a mockEmbedder pointer can be assigned to an Embedder var.
		var iface Embedder = m
		_, ok := iface.(*mockEmbedder)
		assert.True(t, ok, "mockEmbedder should satisfy Embedder interface")
	})
}