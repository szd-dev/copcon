package kbworker

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	knowledgebase "github.com/copcon/plugins/knowledge-base"
	kbrag "github.com/copcon/plugins/knowledge-base/rag"
	kbtypes "github.com/copcon/plugins/knowledge-base/types"
)

type DocumentWorker struct {
	store    knowledgebase.KnowledgeStore
	embedder kbtypes.Embedder
	interval time.Duration
	stopCh   chan struct{}
	wg       sync.WaitGroup
	logger   *slog.Logger
}

func New(store knowledgebase.KnowledgeStore, embedder kbtypes.Embedder, interval time.Duration) *DocumentWorker {
	return &DocumentWorker{
		store:    store,
		embedder: embedder,
		interval: interval,
		stopCh:   make(chan struct{}),
		logger:   slog.Default(),
	}
}

func (w *DocumentWorker) Start() {
	w.wg.Add(1)
	go w.run()
	w.logger.Info("DocumentWorker started", "interval", w.interval)
}

func (w *DocumentWorker) Stop() {
	close(w.stopCh)
	w.wg.Wait()
	w.logger.Info("DocumentWorker stopped")
}

func (w *DocumentWorker) run() {
	defer w.wg.Done()
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.poll()
		}
	}
}

func (w *DocumentWorker) poll() {
	ctx := context.Background()
	docs, err := w.store.ListDocumentsByStatus(ctx, []string{
		string(kbtypes.DocStatusPending),
		string(kbtypes.DocStatusParsing),
		string(kbtypes.DocStatusIndexing),
	})
	if err != nil {
		w.logger.Error("DocumentWorker: failed to list documents", "error", err)
		return
	}
	for _, doc := range docs {
		switch doc.Status {
		case kbtypes.DocStatusPending:
			claimed, err := w.store.ClaimDocumentStatus(ctx, doc.ID, string(kbtypes.DocStatusParsing), string(kbtypes.DocStatusPending))
			if err != nil {
				w.logger.Warn("claim pending failed", "doc", doc.ID, "error", err)
				continue
			}
			if !claimed {
				continue
			}
			w.processParsing(ctx, doc)
		case kbtypes.DocStatusParsing:
			w.processParsing(ctx, doc)
		case kbtypes.DocStatusIndexing:
			w.processIndexing(ctx, doc)
		}
	}
}

func (w *DocumentWorker) processParsing(ctx context.Context, doc *kbtypes.Document) {
	parser := kbrag.NewDefaultParser()
	text, err := parser.Parse(ctx, []byte(doc.Content), "text/plain")
	if err != nil {
		w.markError(ctx, doc, "parse failed", err)
		return
	}

	chunker := kbrag.NewRecursiveChunker()
	chunks, err := chunker.Chunk(text, kbrag.ChunkOptions{ChunkSize: 1000, ChunkOverlap: 200})
	if err != nil {
		w.markError(ctx, doc, "chunk failed", err)
		return
	}

	if err := w.store.UpdateDocumentStatus(ctx, doc.KBID, doc.ID, kbtypes.DocStatusIndexing); err != nil {
		w.logger.Error("failed to update to indexing", "doc", doc.ID, "error", err)
	}

	w.indexChunks(ctx, doc, chunks)
}

func (w *DocumentWorker) processIndexing(ctx context.Context, doc *kbtypes.Document) {
	parser := kbrag.NewDefaultParser()
	text, err := parser.Parse(ctx, []byte(doc.Content), "text/plain")
	if err != nil {
		w.markError(ctx, doc, "re-parse failed", err)
		return
	}

	chunker := kbrag.NewRecursiveChunker()
	chunks, err := chunker.Chunk(text, kbrag.ChunkOptions{ChunkSize: 1000, ChunkOverlap: 200})
	if err != nil {
		w.markError(ctx, doc, "re-chunk failed", err)
		return
	}

	w.indexChunks(ctx, doc, chunks)
}

func (w *DocumentWorker) indexChunks(ctx context.Context, doc *kbtypes.Document, chunks []kbrag.ChunkResult) {
	if w.embedder == nil {
		w.markError(ctx, doc, "embedder not configured", fmt.Errorf("no embedder"))
		return
	}

	if len(chunks) == 0 {
		_ = w.store.UpdateDocumentStatus(ctx, doc.KBID, doc.ID, kbtypes.DocStatusReady)
		return
	}

	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}

	vectors, err := w.embedder.EmbedBatch(ctx, texts)
	if err != nil {
		w.markError(ctx, doc, "embed failed", err)
		return
	}

	if len(vectors) == 0 {
		w.logger.Warn("embedder returned zero vectors without error", "doc", doc.ID, "texts_count", len(texts))
		w.markError(ctx, doc, "embed failed", fmt.Errorf("embedder returned zero vectors for %d texts", len(texts)))
		return
	}

	kbChunks := make([]*kbtypes.Chunk, len(chunks))
	for i, c := range chunks {
		kbChunks[i] = &kbtypes.Chunk{
			DocumentID: doc.ID,
			KBID:       doc.KBID,
			Content:    c.Content,
			Index:      c.Index,
			TokenCount: int(float64(len(c.Content)) / 4.0),
			Metadata:   c.Metadata,
		}
	}

	if err := w.store.StoreChunks(ctx, doc.KBID, doc.ID, kbChunks, vectors); err != nil {
		w.markError(ctx, doc, "store chunks failed", err)
		return
	}

	w.logger.Info("document processed", "doc", doc.ID, "chunks", len(chunks))
}

func (w *DocumentWorker) markError(ctx context.Context, doc *kbtypes.Document, stage string, err error) {
	w.logger.Error("document processing failed", "doc", doc.ID, "stage", stage, "error", err)
	_ = w.store.UpdateDocumentStatus(ctx, doc.KBID, doc.ID, kbtypes.DocStatusError)
	_ = w.store.UpdateDocumentErrorMsg(ctx, doc.KBID, doc.ID, fmt.Sprintf("%s: %v", stage, err))
}
