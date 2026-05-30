package api

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	memtypes "github.com/copcon/plugins/memory-file/types"
)

func (h *Handler) ListAgentMemories(c *gin.Context) {
	agentID := c.Param("agentId")
	if agentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent id is required"})
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

	memories, err := h.memoryStore.GetByAgentID(c.Request.Context(), agentID, limit)
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

func (h *Handler) DeleteAgentMemory(c *gin.Context) {
	agentID := c.Param("agentId")
	memoryID := c.Param("memoryId")
	if agentID == "" || memoryID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent id and memory id are required"})
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

	if mem.AgentID != agentID {
		c.JSON(http.StatusNotFound, gin.H{"error": "memory not found for this agent"})
		return
	}

	if err := h.memoryStore.Delete(c.Request.Context(), memoryID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to delete memory: %s", err.Error())})
		return
	}

	c.Status(http.StatusNoContent)
}

func memoryToJSON(mem *memtypes.Memory) gin.H {
	return gin.H{
		"id":          mem.ID,
		"content":     mem.Content,
		"agent_id":    mem.AgentID,
		"role":        mem.Role,
		"timestamp":   mem.Timestamp,
		"memory_type": mem.MemoryType,
		"metadata":    mem.Metadata,
		"score":       mem.Score,
		"importance":  mem.Importance,
	}
}
