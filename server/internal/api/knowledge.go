package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	kbtypes "github.com/copcon/plugins/knowledge-base/types"
)

func (h *Handler) CreateKB(c *gin.Context) {
	var req struct {
		Name    string         `json:"name" binding:"required"`
		Backend string         `json:"backend"`
		Config  map[string]any `json:"config,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	if h.knowledgeStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "knowledge store not configured"})
		return
	}

	backend := req.Backend
	if backend == "" {
		backend = "sqlite-vec"
	}

	kb, err := h.knowledgeStore.CreateKB(c.Request.Context(), &kbtypes.KnowledgeBase{
		ID:        uuid.New().String(),
		Name:      req.Name,
		Backend:   backend,
		Config:    req.Config,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Metadata:  make(map[string]any),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create knowledge base: %s", err.Error())})
		return
	}

	c.JSON(http.StatusCreated, kbToJSON(kb))
}

func (h *Handler) ListKBs(c *gin.Context) {
	if h.knowledgeStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "knowledge store not configured"})
		return
	}

	kbs, err := h.knowledgeStore.ListKBs(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result := make([]gin.H, len(kbs))
	for i, kb := range kbs {
		result[i] = kbToJSON(kb)
	}

	c.JSON(http.StatusOK, gin.H{"knowledge_bases": result})
}

func (h *Handler) GetKB(c *gin.Context) {
	kbID := c.Param("kbId")
	if kbID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "kb id is required"})
		return
	}

	if h.knowledgeStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "knowledge store not configured"})
		return
	}

	kb, err := h.knowledgeStore.GetKB(c.Request.Context(), kbID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "knowledge base not found"})
		return
	}

	c.JSON(http.StatusOK, kbToJSON(kb))
}

func (h *Handler) DeleteKB(c *gin.Context) {
	kbID := c.Param("kbId")
	if kbID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "kb id is required"})
		return
	}

	if h.knowledgeStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "knowledge store not configured"})
		return
	}

	if err := h.knowledgeStore.DeleteKB(c.Request.Context(), kbID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "knowledge base not found"})
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *Handler) UploadDocument(c *gin.Context) {
	kbID := c.Param("kbId")
	if kbID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "kb id is required"})
		return
	}

	if h.knowledgeStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "knowledge store not configured"})
		return
	}

	_, err := h.knowledgeStore.GetKB(c.Request.Context(), kbID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "knowledge base not found"})
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	defer file.Close()

	content := make([]byte, header.Size)
	if _, err := file.Read(content); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file"})
		return
	}

	mimetype := header.Header.Get("Content-Type")
	if mimetype == "" {
		mimetype = "application/octet-stream"
	}

	doc := &kbtypes.Document{
		ID:         uuid.New().String(),
		KBID:       kbID,
		Filename:   header.Filename,
		Source:     "upload",
		Status:     kbtypes.DocStatusPending,
		ChunkCount: 0,
		TokenCount: 0,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Metadata:   make(map[string]any),
	}

	if h.ragPipeline != nil {
		go func() {
			bgCtx := c.Request.Context()
			if err := h.ragPipeline.Ingest(bgCtx, kbID, doc, content, mimetype, nil); err != nil {
				slog.Default().Error("async document ingestion failed",
					"kb_id", kbID, "doc_id", doc.ID, "error", err)
			}
		}()
	}

	c.JSON(http.StatusAccepted, docToJSON(doc))
}

func (h *Handler) ListDocuments(c *gin.Context) {
	kbID := c.Param("kbId")
	if kbID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "kb id is required"})
		return
	}

	if h.knowledgeStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "knowledge store not configured"})
		return
	}

	docs, err := h.knowledgeStore.ListDocuments(c.Request.Context(), kbID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result := make([]gin.H, len(docs))
	for i, doc := range docs {
		result[i] = docToJSON(doc)
	}

	c.JSON(http.StatusOK, gin.H{"documents": result})
}

func (h *Handler) GetDocument(c *gin.Context) {
	kbID := c.Param("kbId")
	docID := c.Param("docId")
	if kbID == "" || docID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "kb id and doc id are required"})
		return
	}

	if h.knowledgeStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "knowledge store not configured"})
		return
	}

	doc, err := h.knowledgeStore.GetDocument(c.Request.Context(), kbID, docID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "document not found"})
		return
	}

	c.JSON(http.StatusOK, docToJSON(doc))
}

func (h *Handler) DeleteDocument(c *gin.Context) {
	kbID := c.Param("kbId")
	docID := c.Param("docId")
	if kbID == "" || docID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "kb id and doc id are required"})
		return
	}

	if h.knowledgeStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "knowledge store not configured"})
		return
	}

	if err := h.knowledgeStore.DeleteDocument(c.Request.Context(), kbID, docID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "document not found"})
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *Handler) SearchKB(c *gin.Context) {
	kbID := c.Param("kbId")
	if kbID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "kb id is required"})
		return
	}

	if h.knowledgeStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "knowledge store not configured"})
		return
	}

	if h.embedder == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "embedder not configured"})
		return
	}

	var req struct {
		Query               string  `json:"query" binding:"required"`
		TopK                int     `json:"top_k"`
		SimilarityThreshold float32 `json:"similarity_threshold"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query is required"})
		return
	}

	_, err := h.knowledgeStore.GetKB(c.Request.Context(), kbID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "knowledge base not found"})
		return
	}

	vector, err := h.embedder.Embed(c.Request.Context(), req.Query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to embed query: %s", err.Error())})
		return
	}

	topK := req.TopK
	if topK <= 0 {
		topK = 5
	}

	opts := kbtypes.SearchOptions{
		TopK:                topK,
		SimilarityThreshold: req.SimilarityThreshold,
	}

	chunks, err := h.knowledgeStore.Search(c.Request.Context(), []string{kbID}, vector, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("search failed: %s", err.Error())})
		return
	}

	result := make([]gin.H, len(chunks))
	for i, chunk := range chunks {
		result[i] = chunkToJSON(chunk)
	}

	c.JSON(http.StatusOK, gin.H{"results": result})
}

func kbToJSON(kb *kbtypes.KnowledgeBase) gin.H {
	return gin.H{
		"id":         kb.ID,
		"name":       kb.Name,
		"backend":    kb.Backend,
		"config":     kb.Config,
		"created_at": kb.CreatedAt,
		"updated_at": kb.UpdatedAt,
		"metadata":   kb.Metadata,
	}
}

func docToJSON(doc *kbtypes.Document) gin.H {
	return gin.H{
		"id":          doc.ID,
		"kb_id":       doc.KBID,
		"filename":    doc.Filename,
		"source":      doc.Source,
		"status":      doc.Status,
		"chunk_count": doc.ChunkCount,
		"token_count": doc.TokenCount,
		"created_at":  doc.CreatedAt,
		"updated_at":  doc.UpdatedAt,
		"metadata":    doc.Metadata,
	}
}

func chunkToJSON(chunk *kbtypes.Chunk) gin.H {
	return gin.H{
		"id":          chunk.ID,
		"document_id": chunk.DocumentID,
		"kb_id":       chunk.KBID,
		"content":     chunk.Content,
		"context":     chunk.Context,
		"index":       chunk.Index,
		"token_count": chunk.TokenCount,
		"metadata":    chunk.Metadata,
		"score":       chunk.Score,
	}
}
