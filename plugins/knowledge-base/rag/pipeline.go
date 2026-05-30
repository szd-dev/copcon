package kbrag

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	kbtypes "github.com/copcon/plugins/knowledge-base/types"
	knowledgebase "github.com/copcon/plugins/knowledge-base"
)

type ProgressFunc func(stage string, current, total int)

type Pipeline struct {
	parser  Parser
	chunker Chunker
	embedder kbtypes.Embedder
	store   knowledgebase.KnowledgeStore
	logger  *slog.Logger
}

func NewPipeline(parser Parser, embedder kbtypes.Embedder, store knowledgebase.KnowledgeStore) *Pipeline {
	return &Pipeline{
		parser:   parser,
		chunker:  NewRecursiveChunker(),
		embedder: embedder,
		store:    store,
		logger:   slog.Default(),
	}
}

func NewMarkdownPipeline(parser Parser, embedder kbtypes.Embedder, store knowledgebase.KnowledgeStore) *Pipeline {
	return &Pipeline{
		parser:   parser,
		chunker:  NewMarkdownAwareChunker(),
		embedder: embedder,
		store:    store,
		logger:   slog.Default(),
	}
}

func (p *Pipeline) Ingest(ctx context.Context, kbID string, doc *kbtypes.Document, content []byte, mimetype string, progress ProgressFunc) error {
	doc.Status = kbtypes.DocStatusPending
	if err := p.store.IngestDocument(ctx, kbID, doc, content); err != nil {
		return fmt.Errorf("create document record: %w", err)
	}

	if err := p.store.UpdateDocumentStatus(ctx, kbID, doc.ID, kbtypes.DocStatusParsing); err != nil {
		p.logger.Warn("failed to update document status to parsing", "error", err)
	}

	text, err := p.parser.Parse(ctx, content, mimetype)
	if err != nil {
		_ = p.store.UpdateDocumentStatus(ctx, kbID, doc.ID, kbtypes.DocStatusError)
		return fmt.Errorf("parse document: %w", err)
	}
	if progress != nil {
		progress("parse", 1, 4)
	}

	var chunker Chunker = p.chunker
	if isMarkdownMimetype(mimetype) {
		chunker = NewMarkdownAwareChunker()
	}

	chunkOpts := ChunkOptions{
		ChunkSize:    1000,
		ChunkOverlap: 200,
	}
	chunkResults, err := chunker.Chunk(text, chunkOpts)
	if err != nil {
		_ = p.store.UpdateDocumentStatus(ctx, kbID, doc.ID, kbtypes.DocStatusError)
		return fmt.Errorf("chunk document: %w", err)
	}
	if progress != nil {
		progress("chunk", 2, 4)
	}

	chunks := make([]*kbtypes.Chunk, len(chunkResults))
	for i, cr := range chunkResults {
		chunks[i] = &kbtypes.Chunk{
			DocumentID: doc.ID,
			KBID:       kbID,
			Content:    cr.Content,
			Index:      cr.Index,
			TokenCount: estimateTokens(cr.Content),
			Metadata:   cr.Metadata,
		}
	}

	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}

	if len(texts) == 0 {
		if err := p.store.UpdateDocumentStatus(ctx, kbID, doc.ID, kbtypes.DocStatusReady); err != nil {
			p.logger.Warn("failed to update document status", "error", err)
		}
		return nil
	}

	vectors, err := p.embedWithRetry(ctx, texts, 3)
	if err != nil {
		_ = p.store.UpdateDocumentStatus(ctx, kbID, doc.ID, kbtypes.DocStatusError)
		return fmt.Errorf("embed chunks: %w", err)
	}
	if progress != nil {
		progress("embed", 3, 4)
	}

	if err := p.store.StoreChunks(ctx, kbID, doc.ID, chunks, vectors); err != nil {
		_ = p.store.UpdateDocumentStatus(ctx, kbID, doc.ID, kbtypes.DocStatusError)
		return fmt.Errorf("store chunks: %w", err)
	}
	if progress != nil {
		progress("store", 4, 4)
	}

	return nil
}

func (p *Pipeline) embedWithRetry(ctx context.Context, texts []string, maxAttempts int) ([][]float32, error) {
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			p.logger.Warn("retrying embedding", "attempt", attempt+1, "backoff", backoff)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		vectors, err := p.embedder.EmbedBatch(ctx, texts)
		if err == nil {
			return vectors, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("after %d attempts: %w", maxAttempts, lastErr)
}

func isMarkdownMimetype(mimetype string) bool {
	return strings.Contains(mimetype, "markdown")
}

func estimateTokens(text string) int {
	return int(float64(len(text)) / 4.0)
}
