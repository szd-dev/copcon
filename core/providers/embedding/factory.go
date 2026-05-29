package embedding

import (
	"fmt"

	"github.com/copcon/core/llm"
)

// NewFromConfig creates an Embedder from the given configuration and LLM provider.
// It dispatches to the appropriate backend implementation based on cfg.Backend.
func NewFromConfig(cfg EmbeddingConfig, llm llm.LLMProvider) (Embedder, error) {
	switch cfg.Backend {
	case BackendOpenAI:
		return NewOpenAIEmbedder(llm, cfg.BaseURL, cfg.APIKey, cfg.OpenAIModel)
	case BackendBGEM3:
		return nil, fmt.Errorf("%w: bge_m3", ErrUnsupportedBackend)
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedBackend, cfg.Backend)
	}
}