package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/copcon/server/internal/config"
)
type mockMCPProvider struct {
	servers      []MCPServerInfo
	serverDetail *MCPServerInfo
	addResult    *MCPServerInfo
	setErr       error
	removeErr    error
	addErr       error
}

func (m *mockMCPProvider) ListServers() ([]MCPServerInfo, error) {
	return m.servers, nil
}

func (m *mockMCPProvider) GetServer(name string) (*MCPServerInfo, error) {
	if m.serverDetail == nil {
		return nil, fmt.Errorf("server %q not found", name)
	}
	return m.serverDetail, nil
}

func (m *mockMCPProvider) AddServer(req MCPServerCreateRequest) (*MCPServerInfo, error) {
	if m.addErr != nil {
		return nil, m.addErr
	}
	if m.addResult != nil {
		return m.addResult, nil
	}
	return &MCPServerInfo{
		Name:    req.Name,
		Type:    req.Type,
		Command: req.Command,
		Args:    req.Args,
		URL:     req.URL,
		Enabled: true,
	}, nil
}

func (m *mockMCPProvider) RemoveServer(name string) error {
	if m.removeErr != nil {
		return m.removeErr
	}
	return nil
}

func (m *mockMCPProvider) SetServerEnabled(name string, enabled bool) error {
	if m.setErr != nil {
		return m.setErr
	}
	return nil
}

func setupMCPTestHandler(t *testing.T, provider MCPProvider) *Handler {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{DefaultAgentID: "default-agent"}
	sessionStore := newMockSessionStore()
	todoStore := &mockTodoStore{}
	messageStore := &mockMessageStore{}
	agentRegistry := newMockAgentRegistry("default-agent")
	harness := &testHarness{
		store:         &testStoreProvider{sessionStore, messageStore, todoStore},
		agentRegistry: agentRegistry,
	}
	handler := NewHandler(cfg, harness)
	if provider != nil {
		handler.mcpProvider = provider
	}
	return handler
}

func setupMCPRouter(h *Handler) *gin.Engine {
	r := gin.New()
	r.GET("/api/mcp/servers", h.ListMCPServers)
	r.GET("/api/mcp/servers/:name", h.GetMCPServer)
	r.POST("/api/mcp/servers", h.AddMCPServer)
	r.DELETE("/api/mcp/servers/:name", h.RemoveMCPServer)
	r.POST("/api/mcp/servers/:name/enable", h.EnableMCPServer)
	r.POST("/api/mcp/servers/:name/disable", h.DisableMCPServer)
	return r
}

func TestMCP_List(t *testing.T) {
	provider := &mockMCPProvider{
		servers: []MCPServerInfo{
			{Name: "fs", Type: "stdio", Command: "mcp-fs", Enabled: true},
			{Name: "git", Type: "sse", URL: "http://localhost:3000", Enabled: false},
		},
	}
	handler := setupMCPTestHandler(t, provider)
	router := setupMCPRouter(handler)

	req, _ := http.NewRequest("GET", "/api/mcp/servers", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	servers, ok := resp["servers"].([]interface{})
	require.True(t, ok)
	assert.Len(t, servers, 2)
}

func TestMCP_List_NotConfigured(t *testing.T) {
	handler := setupMCPTestHandler(t, nil)
	router := setupMCPRouter(handler)

	req, _ := http.NewRequest("GET", "/api/mcp/servers", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp["error"], "mcp plugin not configured")
}

func TestMCP_Get(t *testing.T) {
	provider := &mockMCPProvider{
		serverDetail: &MCPServerInfo{
			Name: "fs", Type: "stdio", Command: "mcp-fs", Enabled: true,
			Tools: []string{"read_file", "write_file"},
		},
	}
	handler := setupMCPTestHandler(t, provider)
	router := setupMCPRouter(handler)

	req, _ := http.NewRequest("GET", "/api/mcp/servers/fs", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp MCPServerInfo
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "fs", resp.Name)
	assert.Equal(t, "stdio", resp.Type)
	assert.True(t, resp.Enabled)
}

func TestMCP_Get_NotFound(t *testing.T) {
	provider := &mockMCPProvider{serverDetail: nil}
	handler := setupMCPTestHandler(t, provider)
	router := setupMCPRouter(handler)

	req, _ := http.NewRequest("GET", "/api/mcp/servers/nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp["error"], "not found")
}

func TestMCP_Add(t *testing.T) {
	provider := &mockMCPProvider{}
	handler := setupMCPTestHandler(t, provider)
	router := setupMCPRouter(handler)

	reqBody := MCPServerCreateRequest{
		Name:    "fs",
		Type:    "stdio",
		Command: "mcp-fs",
		Args:    []string{"--root", "/tmp"},
	}
	jsonBody, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest("POST", "/api/mcp/servers", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp MCPServerInfo
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "fs", resp.Name)
	assert.Equal(t, "stdio", resp.Type)
	assert.Equal(t, "mcp-fs", resp.Command)
	assert.True(t, resp.Enabled)
}

func TestMCP_Add_InvalidBody(t *testing.T) {
	provider := &mockMCPProvider{}
	handler := setupMCPTestHandler(t, provider)
	router := setupMCPRouter(handler)

	req, _ := http.NewRequest("POST", "/api/mcp/servers", bytes.NewBufferString("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp["error"], "invalid request body")
}

func TestMCP_Remove(t *testing.T) {
	provider := &mockMCPProvider{}
	handler := setupMCPTestHandler(t, provider)
	router := setupMCPRouter(handler)

	req, _ := http.NewRequest("DELETE", "/api/mcp/servers/fs", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestMCP_Remove_NotFound(t *testing.T) {
	provider := &mockMCPProvider{
		removeErr: fmt.Errorf("server \"ghost\" not found"),
	}
	handler := setupMCPTestHandler(t, provider)
	router := setupMCPRouter(handler)

	req, _ := http.NewRequest("DELETE", "/api/mcp/servers/ghost", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp["error"], "not found")
}

func TestMCP_Enable(t *testing.T) {
	provider := &mockMCPProvider{}
	handler := setupMCPTestHandler(t, provider)
	router := setupMCPRouter(handler)

	req, _ := http.NewRequest("POST", "/api/mcp/servers/fs/enable", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "fs", resp["name"])
	assert.Equal(t, true, resp["enabled"])
}

func TestMCP_Disable(t *testing.T) {
	provider := &mockMCPProvider{}
	handler := setupMCPTestHandler(t, provider)
	router := setupMCPRouter(handler)

	req, _ := http.NewRequest("POST", "/api/mcp/servers/fs/disable", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "fs", resp["name"])
	assert.Equal(t, false, resp["enabled"])
}
