package testutil

import (
	"context"
	"fmt"

	kbtypes "github.com/copcon/plugins/knowledge-base/types"
)

var errEmptyText = fmt.Errorf("empty text provided for embedding")

type MockEmbedder struct {
	Dims         int
	EmbedderName string
}

func (m *MockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return nil, errEmptyText
	}
	vec := make([]float32, m.Dims)
	vec[0] = 1.0
	return vec, nil
}

func (m *MockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, errEmptyText
	}
	results := make([][]float32, len(texts))
	for i, text := range texts {
		if text == "" {
			return nil, errEmptyText
		}
		vec := make([]float32, m.Dims)
		vec[0] = float32(i) / float32(len(texts))
		results[i] = vec
	}
	return results, nil
}

func (m *MockEmbedder) Dimensions() int {
	if m.Dims == 0 {
		return 3
	}
	return m.Dims
}

func (m *MockEmbedder) Name() string {
	if m.EmbedderName == "" {
		return "mock"
	}
	return m.EmbedderName
}

var _ kbtypes.Embedder = (*MockEmbedder)(nil)
