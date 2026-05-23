package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/copcon/core/agent"
	"github.com/copcon/core/chatcontext"
	"github.com/copcon/server/internal/config"
	"github.com/copcon/core/entity"
	"github.com/copcon/core/iface"
	"github.com/copcon/server/internal/session"
	"github.com/copcon/core/storage"
	"github.com/copcon/server/internal/tools/todo"
)

// SessionAgentStore stores active ChatContext instances keyed by session ID.
// It is used to reconnect to an in-progress agent session.
type SessionAgentStore struct {
	mu       sync.RWMutex
	contexts map[string]iface.ChatContextInterface
}

func NewSessionAgentStore() *SessionAgentStore {
	return &SessionAgentStore{
		contexts: make(map[string]iface.ChatContextInterface),
	}
}

func (s *SessionAgentStore) Put(sessionID string, ctx iface.ChatContextInterface) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.contexts[sessionID] = ctx
}

func (s *SessionAgentStore) Get(sessionID string) (iface.ChatContextInterface, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ctx, ok := s.contexts[sessionID]
	return ctx, ok
}

func (s *SessionAgentStore) Remove(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.contexts, sessionID)
}

type Handler struct {
	config            *config.Config
	sessionMgr        session.SessionManager
	todoMgr           todo.TodoManager
	agent             agent.AgentEngine
	agentRegistry     agent.AgentRegistry
	sessionAgentStore *SessionAgentStore
	messageStore      storage.MessageStore
}

func NewHandler(cfg *config.Config, sessionMgr session.SessionManager, todoMgr todo.TodoManager, agentEngine agent.AgentEngine, agentRegistry agent.AgentRegistry, messageStore storage.MessageStore) *Handler {
	return &Handler{
		config:            cfg,
		sessionMgr:        sessionMgr,
		todoMgr:           todoMgr,
		agent:             agentEngine,
		agentRegistry:     agentRegistry,
		sessionAgentStore: NewSessionAgentStore(),
		messageStore:      messageStore,
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

	chatCtx := chatcontext.NewChatContext(c.Request.Context(), "", defaultAgentID)
	sess, err := h.sessionMgr.CreateSession(chatCtx, title, defaultAgentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	chatCtxForCount := chatcontext.NewChatContext(c.Request.Context(), sess.ID.String(), "")
	count, _ := h.sessionMgr.GetSessionMessageCount(chatCtxForCount)

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

	chatCtx := chatcontext.NewChatContext(c.Request.Context(), "", "")
	sessions, total, err := h.sessionMgr.ListSessions(chatCtx, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result := make([]gin.H, len(sessions))
	for i, sess := range sessions {
		chatCtxForCount := chatcontext.NewChatContext(c.Request.Context(), sess.ID.String(), "")
		count, _ := h.sessionMgr.GetSessionMessageCount(chatCtxForCount)
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

	chatCtx := chatcontext.NewChatContext(c.Request.Context(), sessionID, "")
	sess, err := h.sessionMgr.GetSession(chatCtx)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	count, _ := h.sessionMgr.GetSessionMessageCount(chatCtx)

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

	chatCtx := chatcontext.NewChatContext(c.Request.Context(), sessionID, "")
	if err := h.sessionMgr.DeleteSession(chatCtx); err != nil {
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

	// Convert to session.Message to reuse BackfillParts/GroupPartsByStep
	messages := make([]session.Message, len(storageMsgs))
	for i, sm := range storageMsgs {
		messages[i] = *session.MessageFromStorage(sm)
	}

	toolResults := make(map[string]string)
	for _, msg := range messages {
		if msg.Role == "tool" && msg.ToolCallID != "" {
			toolResults[msg.ToolCallID] = msg.Content
		}
	}

	var filtered []session.Message
	for _, msg := range messages {
		if msg.Role != "tool" {
			filtered = append(filtered, msg)
		}
	}

	result := make([]gin.H, len(filtered))
	for i, msg := range filtered {
		parts := msg.Parts
		if len(parts) == 0 {
			parts = session.BackfillParts(msg, toolResults)
		}

		steps := session.GroupPartsByStep(parts)

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
	sessionID := c.Param("sessionId")

	var req struct {
		Content      string `json:"content"`
		AgentID      string `json:"agent_id"`
		Reconnect    bool   `json:"reconnect"`
		LastEventSeq int64  `json:"last_event_seq"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
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

	var chatCtx iface.ChatContextInterface
	var sub *iface.Subscriber

	if req.Reconnect {
		// Reconnect path: resume SSE from existing active agent
		var found bool
		chatCtx, found = h.sessionAgentStore.Get(sessionID)
		if !found {
			c.Status(http.StatusNoContent)
			return
		}
		fromSeq := req.LastEventSeq + 1
		sub, found = chatCtx.Subscribe(fromSeq)
		if !found {
			data, _ := json.Marshal(entity.Event{Type: "events_lost"})
			fmt.Fprintf(c.Writer, "data: %s\n\n", data)
			flusher.Flush()
			return
		}
	} else {
		// First-connect path: validate, create context, start agent
		if req.Content == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "content is required"})
			return
		}

		if _, active := h.sessionAgentStore.Get(sessionID); active {
			c.JSON(http.StatusConflict, gin.H{"error": "session already has an active agent"})
			return
		}

		chatCtx = chatcontext.NewChatContext(c.Request.Context(), sessionID, req.AgentID)
		h.sessionAgentStore.Put(sessionID, chatCtx)

		go func() {
			defer func() {
				h.sessionAgentStore.Remove(sessionID)
				chatCtx.Close()
			}()
			if err := h.agent.Chat(chatCtx, req.Content); err != nil {
				slog.Error("Agent chat error", "session_id", sessionID, "error", err)
			}
		}()

		var ok bool
		sub, ok = chatCtx.Subscribe(0)
		if !ok {
			data, _ := json.Marshal(entity.Event{Type: "events_lost"})
			fmt.Fprintf(c.Writer, "data: %s\n\n", data)
			flusher.Flush()
			return
		}
	}

	// Unified SSE loop — used by both first-connect and reconnect paths
	for {
		select {
		case event, ok := <-sub.Events:
			if !ok {
				return // ringbuf closed = agent ended
			}
			data, _ := json.Marshal(event)
			_, err := fmt.Fprintf(c.Writer, "data: %s\n\n", data)
			if err != nil {
				return // write failed = client disconnected
			}
			flusher.Flush()
		case <-c.Request.Context().Done():
			return // HTTP context canceled = client disconnected, just clean up subscriber
		}
	}
}

func (h *Handler) StopSession(c *gin.Context) {
	sessionID := c.Param("sessionId")
	chatCtx, found := h.sessionAgentStore.Get(sessionID)
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

	chatCtx, found := h.sessionAgentStore.Get(sessionID)
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

func SetupRoutes(r *gin.Engine, cfg *config.Config, sessionMgr session.SessionManager, todoMgr todo.TodoManager, agentEngine agent.AgentEngine, agentRegistry agent.AgentRegistry, messageStore storage.MessageStore) {
	handler := NewHandler(cfg, sessionMgr, todoMgr, agentEngine, agentRegistry, messageStore)

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
	}
}

func (h *Handler) GetSessionUpdates(c *gin.Context) {
	sessionID := c.Param("sessionId")
	lastEventID := c.Query("since")

	chatCtx := chatcontext.NewChatContext(c.Request.Context(), sessionID, "")
	session, err := h.sessionMgr.GetSession(chatCtx)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	var events []map[string]any
	if session.Metadata != nil {
		if pending, ok := session.Metadata["async_completion_pending"].([]map[string]any); ok {
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

	chatCtx := chatcontext.NewChatContext(c.Request.Context(), sessionID, "")
	_, err := h.sessionMgr.GetSession(chatCtx)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	todos, err := h.todoMgr.ListTodos(chatCtx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve todos"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"todos": todos})
}
