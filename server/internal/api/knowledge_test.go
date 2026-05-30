package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/copcon/core/agent"
	"github.com/copcon/core/chat"
	"github.com/copcon/core/storage"
	"github.com/copcon/server/internal/config"
)

type mockKnowledgeStore struct {
	kbs  map[string]*storage.KnowledgeBase
	docs map[string]map[string]*storage.Document
}

func newMockKnowledgeStore() *mockKnowledgeStore {
	return &mockKnowledgeStore{
		kbs:  make(map[string]*storage.KnowledgeBase),
		docs: make(map[string]map[string]*storage.Document),
	}
}

func (m *mockKnowledgeStore) CreateKB(_ context.Context, kb *storage.KnowledgeBase) (*storage.KnowledgeBase, error) {
	if kb.ID == "" {
		kb.ID = uuid.New().String()
	}
	m.kbs[kb.ID] = kb
	m.docs[kb.ID] = make(map[string]*storage.Document)
	return kb, nil
}

func (m *mockKnowledgeStore) DeleteKB(_ context.Context, id string) error {
	if _, ok := m.kbs[id]; !ok {
		return fmt.Errorf("knowledge base not found")
	}
	delete(m.kbs, id)
	delete(m.docs, id)
	return nil
}

func (m *mockKnowledgeStore) ListKBs(_ context.Context) ([]*storage.KnowledgeBase, error) {
	var list []*storage.KnowledgeBase
	for _, kb := range m.kbs {
		list = append(list, kb)
	}
	return list, nil
}

func (m *mockKnowledgeStore) GetKB(_ context.Context, id string) (*storage.KnowledgeBase, error) {
	kb, ok := m.kbs[id]
	if !ok {
		return nil, fmt.Errorf("knowledge base not found")
	}
	return kb, nil
}

func (m *mockKnowledgeStore) IngestDocument(_ context.Context, kbID string, doc *storage.Document, _ []byte) error {
	if _, ok := m.kbs[kbID]; !ok {
		return fmt.Errorf("knowledge base not found")
	}
	if doc.ID == "" {
		doc.ID = uuid.New().String()
	}
	doc.KBID = kbID
	m.docs[kbID][doc.ID] = doc
	return nil
}

func (m *mockKnowledgeStore) ListDocuments(_ context.Context, kbID string) ([]*storage.Document, error) {
	docs, ok := m.docs[kbID]
	if !ok {
		return nil, fmt.Errorf("knowledge base not found")
	}
	var list []*storage.Document
	for _, doc := range docs {
		list = append(list, doc)
	}
	return list, nil
}

func (m *mockKnowledgeStore) DeleteDocument(_ context.Context, kbID string, docID string) error {
	docs, ok := m.docs[kbID]
	if !ok {
		return fmt.Errorf("knowledge base not found")
	}
	if _, ok := docs[docID]; !ok {
		return fmt.Errorf("document not found")
	}
	delete(docs, docID)
	return nil
}

func (m *mockKnowledgeStore) GetDocument(_ context.Context, kbID string, docID string) (*storage.Document, error) {
	docs, ok := m.docs[kbID]
	if !ok {
		return nil, fmt.Errorf("knowledge base not found")
	}
	doc, ok := docs[docID]
	if !ok {
		return nil, fmt.Errorf("document not found")
	}
	return doc, nil
}

func (m *mockKnowledgeStore) GetChunks(_ context.Context, kbID string, docID string) ([]*storage.Chunk, error) {
	return nil, nil
}

func (m *mockKnowledgeStore) UpdateChunk(_ context.Context, kbID string, chunk *storage.Chunk) error {
	return nil
}

func (m *mockKnowledgeStore) Search(_ context.Context, kbIDs []string, query []float32, opts storage.SearchOptions) ([]*storage.Chunk, error) {
	return nil, nil
}

type mockMemoryStore struct {
	memories map[string]*storage.Memory
}

func newMockMemoryStore() *mockMemoryStore {
	return &mockMemoryStore{
		memories: make(map[string]*storage.Memory),
	}
}

func (m *mockMemoryStore) Store(_ context.Context, memory *storage.Memory) error {
	if memory.ID == "" {
		memory.ID = uuid.New().String()
	}
	m.memories[memory.ID] = memory
	return nil
}

func (m *mockMemoryStore) Search(_ context.Context, query []float32, limit int) ([]*storage.Memory, error) {
	return nil, nil
}

func (m *mockMemoryStore) GetBySession(_ context.Context, sessionID string, limit int) ([]*storage.Memory, error) {
	var result []*storage.Memory
	for _, mem := range m.memories {
		if mem.SessionID == sessionID {
			result = append(result, mem)
		}
	}
	return result, nil
}

func (m *mockMemoryStore) DeleteBySession(_ context.Context, sessionID string) error {
	for id, mem := range m.memories {
		if mem.SessionID == sessionID {
			delete(m.memories, id)
		}
	}
	return nil
}

func (m *mockMemoryStore) List(_ context.Context, filter storage.MemoryFilter) ([]*storage.Memory, error) {
	return nil, nil
}

func (m *mockMemoryStore) Get(_ context.Context, id string) (*storage.Memory, error) {
	mem, ok := m.memories[id]
	if !ok {
		return nil, fmt.Errorf("memory not found")
	}
	return mem, nil
}

func (m *mockMemoryStore) Update(_ context.Context, memory *storage.Memory) error {
	m.memories[memory.ID] = memory
	return nil
}

func (m *mockMemoryStore) Delete(_ context.Context, id string) error {
	if _, ok := m.memories[id]; !ok {
		return fmt.Errorf("memory not found")
	}
	delete(m.memories, id)
	return nil
}

type mockEmbedder struct {
	dimensions int
}

func (e *mockEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	vec := make([]float32, e.dimensions)
	for i := range vec {
		vec[i] = 0.1
	}
	return vec, nil
}

func (e *mockEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i := range texts {
		vec := make([]float32, e.dimensions)
		for j := range vec {
			vec[j] = 0.1
		}
		results[i] = vec
	}
	return results, nil
}

func (e *mockEmbedder) Dimensions() int { return e.dimensions }
func (e *mockEmbedder) Name() string    { return "mock-embedder" }

type kbTestStoreProvider struct {
	sessionStore   *mockSessionStore
	messageStore   *mockMessageStore
	todoStore      *mockTodoStore
	knowledgeStore *mockKnowledgeStore
}

func (p *kbTestStoreProvider) Sessions() storage.SessionStore    { return p.sessionStore }
func (p *kbTestStoreProvider) Messages() storage.MessageStore    { return p.messageStore }
func (p *kbTestStoreProvider) Todos() storage.TodoStore          { return p.todoStore }

type kbTestHarness struct {
	store         *kbTestStoreProvider
	engine        agent.AgentEngine
	agentRegistry agent.AgentRegistry
}

func (h *kbTestHarness) Store() storage.StoreProvider        { return h.store }
func (h *kbTestHarness) Engine() agent.AgentEngine            { return h.engine }
func (h *kbTestHarness) Registry() agent.AgentRegistry        { return h.agentRegistry }
func (h *kbTestHarness) ActiveSessions() chat.ActiveSessions  { return chat.NewActiveSessions() }

func setupKBTestHandler(t *testing.T) (*Handler, *mockKnowledgeStore, *mockMemoryStore, *gin.Engine) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		DefaultAgentID: "default-agent",
		Agents:         []config.AgentConfig{{ID: "default-agent", Name: "Default"}},
	}

	ks := newMockKnowledgeStore()
	ms := newMockMemoryStore()
	sessionStore := newMockSessionStore()
	todoStore := &mockTodoStore{}
	messageStore := &mockMessageStore{}
	agentRegistry := newMockAgentRegistry("default-agent")
	agentRegistry.agents["default-agent"] = agent.AgentDefinition{ID: "default-agent", Name: "Default", Model: "gpt-4o"}

	h := &kbTestHarness{
		store:         &kbTestStoreProvider{sessionStore, messageStore, todoStore, ks},
		agentRegistry: agentRegistry,
	}

	handler := NewHandler(cfg, h,
		WithKnowledgeStore(ks),
		WithMemoryStore(ms),
		WithEmbedder(&mockEmbedder{dimensions: 8}),
	)

	r := gin.New()
	api := r.Group("/api")
	{
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

		sessions := api.Group("/sessions")
		{
			sessions.POST("", handler.CreateSession)
			sessions.GET("/:sessionId/memories", handler.ListSessionMemories)
			sessions.DELETE("/:sessionId/memories/:memoryId", handler.DeleteSessionMemory)
		}
	}

	return handler, ks, ms, r
}

func TestCreateKB(t *testing.T) {
	_, ks, _, router := setupKBTestHandler(t)

	body := `{"name":"Test KB","backend":"sqlite-vec"}`
	req, _ := http.NewRequest("POST", "/api/kb", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "Test KB", resp["name"])
	assert.Equal(t, "sqlite-vec", resp["backend"])
	assert.NotEmpty(t, resp["id"])

	kbs, _ := ks.ListKBs(context.Background())
	assert.Len(t, kbs, 1)
}

func TestCreateKBMissingName(t *testing.T) {
	_, _, _, router := setupKBTestHandler(t)

	body := `{"backend":"sqlite-vec"}`
	req, _ := http.NewRequest("POST", "/api/kb", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestListKBs(t *testing.T) {
	_, _, _, router := setupKBTestHandler(t)

	_, ks, _, _ := setupKBTestHandler(t)

	ks.CreateKB(context.Background(), &storage.KnowledgeBase{Name: "KB1", Backend: "sqlite-vec", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	ks.CreateKB(context.Background(), &storage.KnowledgeBase{Name: "KB2", Backend: "sqlite-vec", CreatedAt: time.Now(), UpdatedAt: time.Now()})

	req, _ := http.NewRequest("GET", "/api/kb", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetKB(t *testing.T) {
	_, ks, _, router := setupKBTestHandler(t)

	kb, _ := ks.CreateKB(context.Background(), &storage.KnowledgeBase{Name: "Test KB", Backend: "sqlite-vec", CreatedAt: time.Now(), UpdatedAt: time.Now()})

	req, _ := http.NewRequest("GET", "/api/kb/"+kb.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, kb.ID, resp["id"])
	assert.Equal(t, "Test KB", resp["name"])
}

func TestGetKBNotFound(t *testing.T) {
	_, _, _, router := setupKBTestHandler(t)

	req, _ := http.NewRequest("GET", "/api/kb/nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDeleteKB(t *testing.T) {
	_, ks, _, router := setupKBTestHandler(t)

	kb, _ := ks.CreateKB(context.Background(), &storage.KnowledgeBase{Name: "To Delete", Backend: "sqlite-vec", CreatedAt: time.Now(), UpdatedAt: time.Now()})

	req, _ := http.NewRequest("DELETE", "/api/kb/"+kb.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	_, err := ks.GetKB(context.Background(), kb.ID)
	assert.Error(t, err)
}

func TestDeleteKBCascade(t *testing.T) {
	_, ks, _, router := setupKBTestHandler(t)

	kb, _ := ks.CreateKB(context.Background(), &storage.KnowledgeBase{Name: "Cascade KB", Backend: "sqlite-vec", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	ks.IngestDocument(context.Background(), kb.ID, &storage.Document{Filename: "test.txt", Source: "upload", Status: storage.DocStatusPending, CreatedAt: time.Now(), UpdatedAt: time.Now()}, []byte("content"))

	req, _ := http.NewRequest("DELETE", "/api/kb/"+kb.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	docs, _ := ks.ListDocuments(context.Background(), kb.ID)
	assert.Nil(t, docs)
}

func TestDeleteKBNotFound(t *testing.T) {
	_, _, _, router := setupKBTestHandler(t)

	req, _ := http.NewRequest("DELETE", "/api/kb/nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUploadDocument(t *testing.T) {
	_, ks, _, router := setupKBTestHandler(t)

	kb, _ := ks.CreateKB(context.Background(), &storage.KnowledgeBase{Name: "Upload KB", Backend: "sqlite-vec", CreatedAt: time.Now(), UpdatedAt: time.Now()})

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "test.txt")
	require.NoError(t, err)
	_, err = part.Write([]byte("hello world"))
	require.NoError(t, err)
	writer.Close()

	req, _ := http.NewRequest("POST", "/api/kb/"+kb.ID+"/docs", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)

	var resp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "test.txt", resp["filename"])
	assert.Equal(t, "upload", resp["source"])
	assert.Equal(t, string(storage.DocStatusPending), resp["status"])
}

func TestUploadDocumentNoFile(t *testing.T) {
	_, ks, _, router := setupKBTestHandler(t)

	kb, _ := ks.CreateKB(context.Background(), &storage.KnowledgeBase{Name: "Upload KB", Backend: "sqlite-vec", CreatedAt: time.Now(), UpdatedAt: time.Now()})

	req, _ := http.NewRequest("POST", "/api/kb/"+kb.ID+"/docs", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUploadDocumentKBNotFound(t *testing.T) {
	_, _, _, router := setupKBTestHandler(t)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", "test.txt")
	part.Write([]byte("hello"))
	writer.Close()

	req, _ := http.NewRequest("POST", "/api/kb/nonexistent/docs", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestListDocuments(t *testing.T) {
	_, ks, _, router := setupKBTestHandler(t)

	kb, _ := ks.CreateKB(context.Background(), &storage.KnowledgeBase{Name: "List Docs KB", Backend: "sqlite-vec", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	ks.IngestDocument(context.Background(), kb.ID, &storage.Document{Filename: "a.txt", Source: "upload", Status: storage.DocStatusReady, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil)
	ks.IngestDocument(context.Background(), kb.ID, &storage.Document{Filename: "b.txt", Source: "upload", Status: storage.DocStatusReady, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil)

	req, _ := http.NewRequest("GET", "/api/kb/"+kb.ID+"/docs", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	docs, ok := resp["documents"].([]interface{})
	require.True(t, ok)
	assert.Len(t, docs, 2)
}

func TestGetDocument(t *testing.T) {
	_, ks, _, router := setupKBTestHandler(t)

	kb, _ := ks.CreateKB(context.Background(), &storage.KnowledgeBase{Name: "Get Doc KB", Backend: "sqlite-vec", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	doc := &storage.Document{ID: uuid.New().String(), Filename: "test.txt", Source: "upload", Status: storage.DocStatusReady, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	ks.IngestDocument(context.Background(), kb.ID, doc, nil)

	req, _ := http.NewRequest("GET", "/api/kb/"+kb.ID+"/docs/"+doc.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "test.txt", resp["filename"])
}

func TestGetDocumentNotFound(t *testing.T) {
	_, ks, _, router := setupKBTestHandler(t)

	kb, _ := ks.CreateKB(context.Background(), &storage.KnowledgeBase{Name: "Get Doc KB", Backend: "sqlite-vec", CreatedAt: time.Now(), UpdatedAt: time.Now()})

	req, _ := http.NewRequest("GET", "/api/kb/"+kb.ID+"/docs/nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDeleteDocument(t *testing.T) {
	_, ks, _, router := setupKBTestHandler(t)

	kb, _ := ks.CreateKB(context.Background(), &storage.KnowledgeBase{Name: "Delete Doc KB", Backend: "sqlite-vec", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	doc := &storage.Document{ID: uuid.New().String(), Filename: "del.txt", Source: "upload", Status: storage.DocStatusReady, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	ks.IngestDocument(context.Background(), kb.ID, doc, nil)

	req, _ := http.NewRequest("DELETE", "/api/kb/"+kb.ID+"/docs/"+doc.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	_, err := ks.GetDocument(context.Background(), kb.ID, doc.ID)
	assert.Error(t, err)
}

func TestDeleteDocumentNotFound(t *testing.T) {
	_, ks, _, router := setupKBTestHandler(t)

	kb, _ := ks.CreateKB(context.Background(), &storage.KnowledgeBase{Name: "Delete Doc KB", Backend: "sqlite-vec", CreatedAt: time.Now(), UpdatedAt: time.Now()})

	req, _ := http.NewRequest("DELETE", "/api/kb/"+kb.ID+"/docs/nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestSearchKB(t *testing.T) {
	_, ks, _, router := setupKBTestHandler(t)

	kb, _ := ks.CreateKB(context.Background(), &storage.KnowledgeBase{Name: "Search KB", Backend: "sqlite-vec", CreatedAt: time.Now(), UpdatedAt: time.Now()})

	body := `{"query":"test query","top_k":5}`
	req, _ := http.NewRequest("POST", "/api/kb/"+kb.ID+"/search", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSearchKBMissingQuery(t *testing.T) {
	_, ks, _, router := setupKBTestHandler(t)

	kb, _ := ks.CreateKB(context.Background(), &storage.KnowledgeBase{Name: "Search KB", Backend: "sqlite-vec", CreatedAt: time.Now(), UpdatedAt: time.Now()})

	body := `{"top_k":5}`
	req, _ := http.NewRequest("POST", "/api/kb/"+kb.ID+"/search", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSearchKBNotFound(t *testing.T) {
	_, _, _, router := setupKBTestHandler(t)

	body := `{"query":"test"}`
	req, _ := http.NewRequest("POST", "/api/kb/nonexistent/search", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestListSessionMemories(t *testing.T) {
	_, _, ms, router := setupKBTestHandler(t)

	sessionID := uuid.New().String()
	ms.Store(context.Background(), &storage.Memory{Content: "memory 1", SessionID: sessionID, Timestamp: time.Now(), MemoryType: "episodic"})
	ms.Store(context.Background(), &storage.Memory{Content: "memory 2", SessionID: sessionID, Timestamp: time.Now(), MemoryType: "semantic"})

	req, _ := http.NewRequest("GET", "/api/sessions/"+sessionID+"/memories", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	memories, ok := resp["memories"].([]interface{})
	require.True(t, ok)
	assert.Len(t, memories, 2)
}

func TestListSessionMemoriesEmpty(t *testing.T) {
	_, _, _, router := setupKBTestHandler(t)

	sessionID := uuid.New().String()

	req, _ := http.NewRequest("GET", "/api/sessions/"+sessionID+"/memories", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	memories, ok := resp["memories"].([]interface{})
	require.True(t, ok)
	assert.Len(t, memories, 0)
}

func TestDeleteSessionMemory(t *testing.T) {
	_, _, ms, router := setupKBTestHandler(t)

	sessionID := uuid.New().String()
	ms.Store(context.Background(), &storage.Memory{Content: "to delete", SessionID: sessionID, Timestamp: time.Now(), MemoryType: "episodic"})

	var memoryID string
	for id := range ms.memories {
		memoryID = id
		break
	}

	req, _ := http.NewRequest("DELETE", "/api/sessions/"+sessionID+"/memories/"+memoryID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestDeleteSessionMemoryWrongSession(t *testing.T) {
	_, _, ms, router := setupKBTestHandler(t)

	sessionID := uuid.New().String()
	otherSessionID := uuid.New().String()
	ms.Store(context.Background(), &storage.Memory{Content: "owned by session1", SessionID: sessionID, Timestamp: time.Now(), MemoryType: "episodic"})

	var memoryID string
	for id := range ms.memories {
		memoryID = id
		break
	}

	req, _ := http.NewRequest("DELETE", "/api/sessions/"+otherSessionID+"/memories/"+memoryID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDeleteSessionMemoryNotFound(t *testing.T) {
	_, _, _, router := setupKBTestHandler(t)

	sessionID := uuid.New().String()

	req, _ := http.NewRequest("DELETE", "/api/sessions/"+sessionID+"/memories/nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestKnowledgeStoreNotConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		DefaultAgentID: "default-agent",
		Agents:         []config.AgentConfig{{ID: "default-agent", Name: "Default"}},
	}

	sessionStore := newMockSessionStore()
	todoStore := &mockTodoStore{}
	messageStore := &mockMessageStore{}
	agentRegistry := newMockAgentRegistry("default-agent")
	agentRegistry.agents["default-agent"] = agent.AgentDefinition{ID: "default-agent", Name: "Default", Model: "gpt-4o"}

	harness := &testHarness{store: &testStoreProvider{sessionStore, messageStore, todoStore}, agentRegistry: agentRegistry}
	handler := NewHandler(cfg, harness)

	r := gin.New()
	kb := r.Group("/api/kb")
	{
		kb.POST("", handler.CreateKB)
		kb.GET("", handler.ListKBs)
		kb.GET("/:kbId", handler.GetKB)
		kb.DELETE("/:kbId", handler.DeleteKB)
	}

	req, _ := http.NewRequest("POST", "/api/kb", bytes.NewBufferString(`{"name":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestMemoryStoreNotConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		DefaultAgentID: "default-agent",
		Agents:         []config.AgentConfig{{ID: "default-agent", Name: "Default"}},
	}

	sessionStore := newMockSessionStore()
	todoStore := &mockTodoStore{}
	messageStore := &mockMessageStore{}
	agentRegistry := newMockAgentRegistry("default-agent")
	agentRegistry.agents["default-agent"] = agent.AgentDefinition{ID: "default-agent", Name: "Default", Model: "gpt-4o"}

	harness := &testHarness{store: &testStoreProvider{sessionStore, messageStore, todoStore}, agentRegistry: agentRegistry}
	handler := NewHandler(cfg, harness)

	r := gin.New()
	r.GET("/api/sessions/:sessionId/memories", handler.ListSessionMemories)

	req, _ := http.NewRequest("GET", "/api/sessions/test-session/memories", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestSetupRoutesConditionalKB(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		DefaultAgentID: "default-agent",
		Agents:         []config.AgentConfig{{ID: "default-agent", Name: "Default"}},
	}

	sessionStore := newMockSessionStore()
	todoStore := &mockTodoStore{}
	messageStore := &mockMessageStore{}
	agentRegistry := newMockAgentRegistry("default-agent")
	agentRegistry.agents["default-agent"] = agent.AgentDefinition{ID: "default-agent", Name: "Default", Model: "gpt-4o"}

	harness := &testHarness{store: &testStoreProvider{sessionStore, messageStore, todoStore}, agentRegistry: agentRegistry}

	r := gin.New()
	SetupRoutes(r, cfg, harness)

	routes := r.Routes()
	hasKBRoutes := false
	for _, route := range routes {
		if len(route.Path) >= 7 && route.Path[:7] == "/api/kb" {
			hasKBRoutes = true
			break
		}
	}
	assert.True(t, hasKBRoutes, "KB routes should always be registered")
}

func TestSetupRoutesWithKB(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		DefaultAgentID: "default-agent",
		Agents:         []config.AgentConfig{{ID: "default-agent", Name: "Default"}},
	}

	ks := newMockKnowledgeStore()
	sessionStore := newMockSessionStore()
	todoStore := &mockTodoStore{}
	messageStore := &mockMessageStore{}
	agentRegistry := newMockAgentRegistry("default-agent")
	agentRegistry.agents["default-agent"] = agent.AgentDefinition{ID: "default-agent", Name: "Default", Model: "gpt-4o"}

	harness := &kbTestHarness{store: &kbTestStoreProvider{sessionStore, messageStore, todoStore, ks}, agentRegistry: agentRegistry}

	r := gin.New()
	SetupRoutes(r, cfg, harness, WithKnowledgeStore(ks), WithEmbedder(&mockEmbedder{dimensions: 8}))

	routes := r.Routes()
	hasKBRoutes := false
	for _, route := range routes {
		if len(route.Path) >= 7 && route.Path[:7] == "/api/kb" {
			hasKBRoutes = true
			break
		}
	}
	assert.True(t, hasKBRoutes, "KB routes should be registered when KnowledgeStore is not nil")
}
