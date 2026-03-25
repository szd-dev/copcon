package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/copcon/server/internal/agent"
	"github.com/copcon/server/internal/config"
	"github.com/copcon/server/internal/session"
)

type Handler struct {
	config     *config.Config
	sessionMgr session.SessionManager
	agent      *agent.AgentEngine
}

func NewHandler(cfg *config.Config, sessionMgr session.SessionManager, agentEngine *agent.AgentEngine) *Handler {
	return &Handler{
		config:     cfg,
		sessionMgr: sessionMgr,
		agent:      agentEngine,
	}
}

func (h *Handler) CreateSession(c *gin.Context) {
	sess, err := h.sessionMgr.Create(c.Request.Context(), "New Chat")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	count, _ := h.sessionMgr.GetMessageCount(c.Request.Context(), sess.ID.String())

	c.JSON(http.StatusCreated, gin.H{
		"id":            sess.ID.String(),
		"title":         sess.Title,
		"created_at":    sess.CreatedAt,
		"updated_at":    sess.UpdatedAt,
		"message_count": count,
	})
}

func (h *Handler) ListSessions(c *gin.Context) {
	limit := 20
	offset := 0

	if l := c.Query("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}
	if o := c.Query("offset"); o != "" {
		fmt.Sscanf(o, "%d", &offset)
	}

	sessions, total, err := h.sessionMgr.List(c.Request.Context(), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result := make([]gin.H, len(sessions))
	for i, sess := range sessions {
		count, _ := h.sessionMgr.GetMessageCount(c.Request.Context(), sess.ID.String())
		result[i] = gin.H{
			"id":            sess.ID.String(),
			"title":         sess.Title,
			"created_at":    sess.CreatedAt,
			"updated_at":    sess.UpdatedAt,
			"message_count": count,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"sessions": result,
		"total":    total,
	})
}

func (h *Handler) GetSession(c *gin.Context) {
	sessionID := c.Param("sessionId")

	sess, err := h.sessionMgr.Get(c.Request.Context(), sessionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	count, _ := h.sessionMgr.GetMessageCount(c.Request.Context(), sessionID)

	c.JSON(http.StatusOK, gin.H{
		"id":            sess.ID.String(),
		"title":         sess.Title,
		"created_at":    sess.CreatedAt,
		"updated_at":    sess.UpdatedAt,
		"message_count": count,
	})
}

func (h *Handler) DeleteSession(c *gin.Context) {
	sessionID := c.Param("sessionId")

	if err := h.sessionMgr.Delete(c.Request.Context(), sessionID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *Handler) GetMessages(c *gin.Context) {
	sessionID := c.Param("sessionId")

	limit := 50
	if l := c.Query("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}

	sessUUID, err := uuid.Parse(sessionID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}

	var messages []session.Message
	db := h.sessionMgr.GetDB()
	if err := db.
		WithContext(c.Request.Context()).
		Where("session_id = ?", sessUUID).
		Order("created_at ASC").
		Limit(limit).
		Find(&messages).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result := make([]gin.H, len(messages))
	for i, msg := range messages {
		result[i] = gin.H{
			"id":           msg.ID.String(),
			"session_id":   msg.SessionID.String(),
			"role":         msg.Role,
			"content":      msg.Content,
			"tool_calls":   msg.ToolCalls,
			"tool_call_id": msg.ToolCallID,
			"created_at":   msg.CreatedAt,
		}
	}

	c.JSON(http.StatusOK, gin.H{"messages": result})
}

func (h *Handler) Chat(c *gin.Context) {
	sessionID := c.Param("sessionId")

	var req struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	events, err := h.agent.Chat(c.Request.Context(), sessionID, req.Content)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming not supported"})
		return
	}

	for event := range events {
		data, _ := json.Marshal(event)
		fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		flusher.Flush()
	}
}

func SetupRoutes(r *gin.Engine, cfg *config.Config, sessionMgr session.SessionManager, agentEngine *agent.AgentEngine) {
	handler := NewHandler(cfg, sessionMgr, agentEngine)

	api := r.Group("/api")
	{
		sessions := api.Group("/sessions")
		{
			sessions.POST("", handler.CreateSession)
			sessions.GET("", handler.ListSessions)
			sessions.GET("/:sessionId", handler.GetSession)
			sessions.DELETE("/:sessionId", handler.DeleteSession)
			sessions.GET("/:sessionId/messages", handler.GetMessages)
			sessions.POST("/:sessionId/chat", handler.Chat)
		}
	}
}
