package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/copcon/server/internal/agent"
	"github.com/copcon/server/internal/config"
	"github.com/copcon/server/internal/domain/iface"
	"github.com/copcon/server/internal/session"
	"github.com/copcon/server/internal/todo"
)

type Handler struct {
	config        *config.Config
	sessionMgr    session.SessionManager
	todoMgr       todo.TodoManager
	agent         *agent.AgentEngine
	agentRegistry agent.AgentRegistry
}

func NewHandler(cfg *config.Config, sessionMgr session.SessionManager, todoMgr todo.TodoManager, agentEngine *agent.AgentEngine, agentRegistry agent.AgentRegistry) *Handler {
	return &Handler{
		config:        cfg,
		sessionMgr:    sessionMgr,
		todoMgr:       todoMgr,
		agent:         agentEngine,
		agentRegistry: agentRegistry,
	}
}

func (h *Handler) CreateSession(c *gin.Context) {
	var req struct {
		Title          string `json:"title"`
		DefaultAgentID string `json:"default_agent_id"`
	}

	// Bind JSON body if present, but allow empty body for backward compatibility
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
	}

	// Use provided title or default
	title := req.Title
	if title == "" {
		title = "New Chat"
	}

	// Use provided agent ID or fall back to config default
	defaultAgentID := req.DefaultAgentID
	if defaultAgentID == "" {
		defaultAgentID = h.config.DefaultAgentID
	}

	chatCtx := iface.NewChatContext(c.Request.Context(), "", defaultAgentID)
	sess, err := h.sessionMgr.Create(chatCtx, title, defaultAgentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	chatCtxForCount := iface.NewChatContext(c.Request.Context(), sess.ID.String(), "")
	count, _ := h.sessionMgr.GetMessageCount(chatCtxForCount)

	c.JSON(http.StatusCreated, gin.H{
		"id":               sess.ID.String(),
		"title":            sess.Title,
		"default_agent_id": sess.DefaultAgentID,
		"created_at":       sess.CreatedAt,
		"updated_at":       sess.UpdatedAt,
		"message_count":    count,
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

	chatCtx := iface.NewChatContext(c.Request.Context(), "", "")
	sessions, total, err := h.sessionMgr.List(chatCtx, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result := make([]gin.H, len(sessions))
	for i, sess := range sessions {
		chatCtxForCount := iface.NewChatContext(c.Request.Context(), sess.ID.String(), "")
		count, _ := h.sessionMgr.GetMessageCount(chatCtxForCount)
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

	chatCtx := iface.NewChatContext(c.Request.Context(), sessionID, "")
	sess, err := h.sessionMgr.Get(chatCtx)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	count, _ := h.sessionMgr.GetMessageCount(chatCtx)

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

	chatCtx := iface.NewChatContext(c.Request.Context(), sessionID, "")
	if err := h.sessionMgr.Delete(chatCtx); err != nil {
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
			"reasoning":    msg.Reasoning,
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
		AgentID string `json:"agent_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	chatCtx := iface.NewChatContext(c.Request.Context(), sessionID, req.AgentID)

	go func() {
		defer chatCtx.Close()
		if err := h.agent.Chat(chatCtx, req.Content); err != nil {
			log.Printf("Agent chat error: %v", err)
		}
	}()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming not supported"})
		return
	}

	for event := range chatCtx.Events() {
		data, _ := json.Marshal(event)
		fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		flusher.Flush()
	}
}

func (h *Handler) ListAgents(c *gin.Context) {
	agents := h.agentRegistry.List()

	result := make([]gin.H, len(agents))
	for i, agent := range agents {
		result[i] = gin.H{
			"id":    agent.ID,
			"name":  agent.Name,
			"model": agent.Model,
		}
	}

	c.JSON(http.StatusOK, gin.H{"agents": result})
}

func SetupRoutes(r *gin.Engine, cfg *config.Config, sessionMgr session.SessionManager, todoMgr todo.TodoManager, agentEngine *agent.AgentEngine, agentRegistry agent.AgentRegistry) {
	handler := NewHandler(cfg, sessionMgr, todoMgr, agentEngine, agentRegistry)

	api := r.Group("/api")
	{
		api.GET("/agents", handler.ListAgents)

		sessions := api.Group("/sessions")
		{
			sessions.POST("", handler.CreateSession)
			sessions.GET("", handler.ListSessions)
			sessions.GET("/:sessionId", handler.GetSession)
			sessions.DELETE("/:sessionId", handler.DeleteSession)
			sessions.GET("/:sessionId/messages", handler.GetMessages)
			sessions.POST("/:sessionId/chat", handler.Chat)
			sessions.GET("/:sessionId/todos", handler.GetSessionTodos)
		}
	}
}

func (h *Handler) GetSessionTodos(c *gin.Context) {
	sessionID := c.Param("sessionId")

	chatCtx := iface.NewChatContext(c.Request.Context(), sessionID, "")
	_, err := h.sessionMgr.Get(chatCtx)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	todos, err := h.todoMgr.List(chatCtx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve todos"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"todos": todos})
}
