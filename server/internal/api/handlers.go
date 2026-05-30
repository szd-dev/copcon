package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/copcon/core"
	"github.com/copcon/core/agent"
	"github.com/copcon/core/chat"
	"github.com/copcon/core/iface"
	"github.com/copcon/core/storage"
	"github.com/copcon/plugins/knowledge-base"
	"github.com/copcon/plugins/memory-file"
	kbrag "github.com/copcon/plugins/knowledge-base/rag"
	"github.com/copcon/server/internal/config"
)

type Handler struct {
	config         *config.Config
	sessionStore   storage.SessionStore
	messageStore   storage.MessageStore
	todoStore      storage.TodoStore
	knowledgeStore knowledgebase.KnowledgeStore
	memoryStore    memoryfile.MemoryStore
	embedder       storage.Embedder
	ragPipeline    *kbrag.Pipeline
	agent          agent.AgentEngine
	agentRegistry  agent.AgentRegistry
	chatStore      chat.ActiveSessions
}

func NewHandler(cfg *config.Config, h core.APIProvider, opts ...HandlerOption) *Handler {
	handler := &Handler{
		config:         cfg,
		sessionStore:   h.Store().Sessions(),
		messageStore:   h.Store().Messages(),
		todoStore:      h.Store().Todos(),
		knowledgeStore: nil,
		agent:          h.Engine(),
		agentRegistry:  h.Registry(),
		chatStore:      h.ActiveSessions(),
	}
	for _, opt := range opts {
		opt(handler)
	}
	return handler
}

type HandlerOption func(*Handler)

func WithMemoryStore(ms memoryfile.MemoryStore) HandlerOption {
	return func(h *Handler) { h.memoryStore = ms }
}

func WithEmbedder(e storage.Embedder) HandlerOption {
	return func(h *Handler) { h.embedder = e }
}

func WithKnowledgeStore(ks knowledgebase.KnowledgeStore) HandlerOption {
	return func(h *Handler) { h.knowledgeStore = ks }
}

func WithRAGPipeline(p *kbrag.Pipeline) HandlerOption {
	return func(h *Handler) { h.ragPipeline = p }
}

func (h *Handler) CreateSession(c *gin.Context) {
	var req struct {
		Title          string `json:"title"`
		DefaultAgentID string `json:"default_agent_id"`
	}

	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
	}

	title := req.Title
	if title == "" {
		title = "New Chat"
	}

	defaultAgentID := req.DefaultAgentID
	if defaultAgentID == "" {
		defaultAgentID = h.config.DefaultAgentID
	}

	sess, err := h.sessionStore.Create(c.Request.Context(), &storage.Session{
		Title:          title,
		DefaultAgentID: defaultAgentID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Metadata:       make(map[string]any),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	count, _ := h.sessionStore.GetMessageCount(c.Request.Context(), sess.ID)

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

	sessions, total, err := h.sessionStore.List(c.Request.Context(), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result := make([]gin.H, len(sessions))
	for i, sess := range sessions {
		count, _ := h.sessionStore.GetMessageCount(c.Request.Context(), sess.ID)
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

	sessUUID, err := uuid.Parse(sessionID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}

	sess, err := h.sessionStore.Get(c.Request.Context(), sessUUID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	count, _ := h.sessionStore.GetMessageCount(c.Request.Context(), sess.ID)

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

	sessUUID, err := uuid.Parse(sessionID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}

	if err := h.sessionStore.Delete(c.Request.Context(), sessUUID); err != nil {
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

	storageMsgs, err := h.messageStore.List(c.Request.Context(), sessUUID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	toolResults := make(map[string]string)
	for _, msg := range storageMsgs {
		if msg.Role == "tool" && msg.ToolCallID != "" {
			toolResults[msg.ToolCallID] = msg.Content
		}
	}

	var filtered []*storage.Message
	for _, msg := range storageMsgs {
		if msg.Role != "tool" {
			filtered = append(filtered, msg)
		}
	}

	result := make([]gin.H, len(filtered))
	for i, msg := range filtered {
		parts := msg.Parts
		if len(parts) == 0 {
			parts = BackfillParts(*msg, toolResults)
		}

		steps := GroupPartsByStep(parts)

		result[i] = gin.H{
			"id":        msg.ID.String(),
			"sessionId": msg.SessionID.String(),
			"role":      msg.Role,
			"steps":     steps,
			"metadata": gin.H{
				"createdAt":  msg.CreatedAt,
				"model":      msg.Model,
				"tokenCount": msg.TokenCount,
				"durationMs": msg.DurationMs,
			},
		}
	}

	c.JSON(http.StatusOK, gin.H{"messages": result})
}

func (h *Handler) Chat(c *gin.Context) {
	var req chat.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	req.SessionID = c.Param("sessionId")

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming not supported"})
		return
	}

	chat.HandleChat(c.Request.Context(), c.Writer, flusher, req, h.agent, h.chatStore)
}

func (h *Handler) StopSession(c *gin.Context) {
	sessionID := c.Param("sessionId")
	chatCtx, found := h.chatStore.Get(sessionID)
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "no active agent for this session"})
		return
	}
	chatCtx.Close()
	c.Status(http.StatusNoContent)
}

type ResumeRequest struct {
	InterruptID string         `json:"interrupt_id" binding:"required"`
	Action      string         `json:"action" binding:"required"`
	Content     map[string]any `json:"content,omitempty"`
}

func (h *Handler) ResumeSession(c *gin.Context) {
	sessionID := c.Param("sessionId")

	var req ResumeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	chatCtx, found := h.chatStore.Get(sessionID)
	if !found {
		c.JSON(http.StatusConflict, gin.H{"error": "no active agent for this session"})
		return
	}

	resp := &iface.InputResponse{
		Action:  req.Action,
		Content: req.Content,
	}

	err := chatCtx.ResolveInput(req.InterruptID, resp)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "resolved"})
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

func SetupRoutes(r *gin.Engine, cfg *config.Config, h core.APIProvider, opts ...HandlerOption) {
	handler := NewHandler(cfg, h, opts...)

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
			sessions.POST("/:sessionId/stop", handler.StopSession)
			sessions.POST("/:sessionId/resume", handler.ResumeSession)
			sessions.GET("/:sessionId/todos", handler.GetSessionTodos)
			sessions.GET("/:sessionId/updates", handler.GetSessionUpdates)
		}

		api.GET("/agents/:agentId/memories", handler.ListAgentMemories)
		api.DELETE("/agents/:agentId/memories/:memoryId", handler.DeleteAgentMemory)

		kb := api.Group("/kb")
		{
			kb.POST("", handler.CreateKB)
			kb.GET("", handler.ListKBs)
			kb.GET("/:kbId", handler.GetKB)
			kb.DELETE("/:kbId", handler.DeleteKB)
			kb.POST("/:kbId/docs", handler.UploadDocument)
			kb.GET("/:kbId/docs", handler.ListDocuments)
			kb.GET("/:kbId/docs/:docId", handler.GetDocument)
			kb.DELETE("/:kbId/docs/:docId", handler.DeleteDocument)
			kb.POST("/:kbId/search", handler.SearchKB)
		}
	}
}

func (h *Handler) GetSessionUpdates(c *gin.Context) {
	sessionID := c.Param("sessionId")
	lastEventID := c.Query("since")

	sessUUID, err := uuid.Parse(sessionID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}

	sess, err := h.sessionStore.Get(c.Request.Context(), sessUUID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	var events []map[string]any
	if sess.Metadata != nil {
		if pending, ok := sess.Metadata["async_completion_pending"].([]map[string]any); ok {
			if lastEventID != "" {
				for _, event := range pending {
					if eventID, ok := event["id"].(string); ok && eventID > lastEventID {
						events = append(events, event)
					}
				}
			} else {
				events = pending
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"has_updates": len(events) > 0,
		"events":      events,
	})
}

func (h *Handler) GetSessionTodos(c *gin.Context) {
	sessionID := c.Param("sessionId")

	sessUUID, err := uuid.Parse(sessionID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}

	_, err = h.sessionStore.Get(c.Request.Context(), sessUUID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	todos, err := h.todoStore.List(c.Request.Context(), sessUUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve todos"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"todos": todos})
}
