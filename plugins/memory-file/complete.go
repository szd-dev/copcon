package memoryfile

import (
	"context"
	"strings"
	"time"

	"github.com/copcon/core/llm"
)

// Complete calls the provider's Stream method and collects all content chunks
// into a single string. If ctx does not already have a deadline, a 30-second
// timeout is applied.
func Complete(ctx context.Context, provider llm.LLMProvider, params llm.StreamParams) (string, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	ch, errc := provider.Stream(ctx, params)

	var sb strings.Builder
	for chunk := range ch {
		sb.WriteString(chunk.Content)
	}

	if err := <-errc; err != nil {
		return "", err
	}

	return sb.String(), nil
}