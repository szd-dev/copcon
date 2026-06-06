package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *Handler) ListMCPServers(c *gin.Context) {
	if h.mcpProvider == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "mcp plugin not configured"})
		return
	}

	servers, err := h.mcpProvider.ListServers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"servers": servers})
}

func (h *Handler) GetMCPServer(c *gin.Context) {
	if h.mcpProvider == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "mcp plugin not configured"})
		return
	}

	name := c.Param("name")
	server, err := h.mcpProvider.GetServer(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, server)
}

func (h *Handler) AddMCPServer(c *gin.Context) {
	if h.mcpProvider == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "mcp plugin not configured"})
		return
	}

	var req MCPServerCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}

	server, err := h.mcpProvider.AddServer(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, server)
}

func (h *Handler) RemoveMCPServer(c *gin.Context) {
	if h.mcpProvider == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "mcp plugin not configured"})
		return
	}

	name := c.Param("name")
	if err := h.mcpProvider.RemoveServer(name); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *Handler) EnableMCPServer(c *gin.Context) {
	if h.mcpProvider == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "mcp plugin not configured"})
		return
	}

	name := c.Param("name")
	if err := h.mcpProvider.SetServerEnabled(name, true); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"name": name, "enabled": true})
}

func (h *Handler) DisableMCPServer(c *gin.Context) {
	if h.mcpProvider == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "mcp plugin not configured"})
		return
	}

	name := c.Param("name")
	if err := h.mcpProvider.SetServerEnabled(name, false); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"name": name, "enabled": false})
}
