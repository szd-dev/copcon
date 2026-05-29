package api

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/copcon/core/storage"
)

func (h *Handler) ListSessionMemories(c *gin.Context) {
	sessionID := c.Param("sessionId")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session id is required"})
		return
	}

	if h.memoryStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "memory store not configured"})
		return
	}

	limit := 50
	if l := c.Query("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}

	memories, err := h.memoryStore.GetBySession(c.Request.Context(), sessionID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result := make([]gin.H, len(memories))
	for i, mem := range memories {
		result[i] = memoryToJSON(mem)
	}

	c.JSON(http.StatusOK, gin.H{"memories": result})
}

func (h *Handler) DeleteSessionMemory(c *gin.Context) {
	sessionID := c.Param("sessionId")
	memoryID := c.Param("memoryId")
	if sessionID == "" || memoryID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session id and memory id are required"})
		return
	}

	if h.memoryStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "memory store not configured"})
		return
	}

	mem, err := h.memoryStore.Get(c.Request.Context(), memoryID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "memory not found"})
		return
	}

	if mem.SessionID != sessionID {
		c.JSON(http.StatusNotFound, gin.H{"error": "memory not found"})
		return
	}

	if err := h.memoryStore.Delete(c.Request.Context(), memoryID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to delete memory: %s", err.Error())})
		return
	}

	c.Status(http.StatusNoContent)
}

func memoryToJSON(mem *storage.Memory) gin.H {
	return gin.H{
		"id":          mem.ID,
		"content":     mem.Content,
		"session_id":  mem.SessionID,
		"role":        mem.Role,
		"timestamp":   mem.Timestamp,
		"memory_type": mem.MemoryType,
		"metadata":    mem.Metadata,
		"score":       mem.Score,
		"importance":  mem.Importance,
	}
}
