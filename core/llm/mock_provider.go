package llm

import "context"

// Compile-time check: MockProvider implements LLMProvider.
var _ LLMProvider = (*MockProvider)(nil)

// MockProvider is a minimal LLMProvider implementation for testing.
// It immediately closes both channels, returning no chunks and no errors.
type MockProvider struct{}

// NewMockProvider creates a MockProvider.
func NewMockProvider() *MockProvider {
	return &MockProvider{}
}

// Stream returns two channels that are immediately closed, simulating
// an empty LLM response with no chunks and no errors.
func (m *MockProvider) Stream(ctx context.Context, params StreamParams) (<-chan StreamChunk, <-chan error) {
	ch := make(chan StreamChunk)
	errc := make(chan error)
	close(ch)
	close(errc)
	return ch, errc
}
