package api

import (
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

type mockSkillProvider struct {
	skills       []SkillInfo
	skillDetails map[string]*SkillDetail
	enabledMap   map[string]bool
	setErr       error
}

func newMockSkillProvider() *mockSkillProvider {
	return &mockSkillProvider{
		skills: []SkillInfo{
			{Name: "coding", Description: "Code assistant", Enabled: true, Source: "builtin"},
			{Name: "review", Description: "Code review", Enabled: false, Source: "builtin"},
		},
		skillDetails: map[string]*SkillDetail{
			"coding": &SkillDetail{
				SkillInfo:   SkillInfo{Name: "coding", Description: "Code assistant", Enabled: true, Source: "builtin"},
				Instructions: "You are a coding assistant.",
				ResourceFiles: []ResourceFileInfo{
					{Name: "style.md", Path: "/skills/coding/style.md", Category: "guide"},
				},
			},
			"review": &SkillDetail{
				SkillInfo:   SkillInfo{Name: "review", Description: "Code review", Enabled: false, Source: "builtin"},
				Instructions: "You review code for quality.",
			},
		},
		enabledMap: map[string]bool{
			"coding": true,
			"review": false,
		},
	}
}

func (m *mockSkillProvider) ListSkills() ([]SkillInfo, error) {
	result := make([]SkillInfo, len(m.skills))
	for i, s := range m.skills {
		result[i] = s
		if enabled, ok := m.enabledMap[s.Name]; ok {
			result[i].Enabled = enabled
		}
	}
	return result, nil
}

func (m *mockSkillProvider) GetSkill(name string) (*SkillDetail, error) {
	detail, ok := m.skillDetails[name]
	if !ok {
		return nil, fmt.Errorf("skill %q not found", name)
	}
	if enabled, ok := m.enabledMap[name]; ok {
		detail.Enabled = enabled
	}
	return detail, nil
}

func (m *mockSkillProvider) SetSkillEnabled(name string, enabled bool) error {
	if m.setErr != nil {
		return m.setErr
	}
	if _, ok := m.skillDetails[name]; !ok {
		return fmt.Errorf("skill %q not found", name)
	}
	m.enabledMap[name] = enabled
	return nil
}

func setupSkillTestHandler(t *testing.T, sp SkillProvider) *Handler {
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
	if sp != nil {
		handler.skillProvider = sp
	}
	return handler
}

func TestSkills_List(t *testing.T) {
	handler := setupSkillTestHandler(t, newMockSkillProvider())

	router := gin.New()
	router.GET("/api/skills", handler.ListSkills)

	req, _ := http.NewRequest("GET", "/api/skills", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	skills, ok := response["skills"].([]interface{})
	require.True(t, ok)
	assert.Len(t, skills, 2)

	skill0 := skills[0].(map[string]interface{})
	assert.Equal(t, "coding", skill0["name"])
	assert.Equal(t, true, skill0["enabled"])

	skill1 := skills[1].(map[string]interface{})
	assert.Equal(t, "review", skill1["name"])
	assert.Equal(t, false, skill1["enabled"])
}

func TestSkills_List_NotConfigured(t *testing.T) {
	handler := setupSkillTestHandler(t, nil)

	router := gin.New()
	router.GET("/api/skills", handler.ListSkills)

	req, _ := http.NewRequest("GET", "/api/skills", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response["error"], "not configured")
}

func TestSkills_Get_WithContent(t *testing.T) {
	handler := setupSkillTestHandler(t, newMockSkillProvider())

	router := gin.New()
	router.GET("/api/skills/:name", handler.GetSkill)

	req, _ := http.NewRequest("GET", "/api/skills/coding?include_content=true", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "coding", response["name"])
	assert.Equal(t, "You are a coding assistant.", response["instructions"])

	resFiles, ok := response["resource_files"].([]interface{})
	require.True(t, ok)
	assert.Len(t, resFiles, 1)

	resFile := resFiles[0].(map[string]interface{})
	assert.Equal(t, "style.md", resFile["name"])
	assert.Equal(t, "guide", resFile["category"])
}

func TestSkills_Get_WithoutContent(t *testing.T) {
	handler := setupSkillTestHandler(t, newMockSkillProvider())

	router := gin.New()
	router.GET("/api/skills/:name", handler.GetSkill)

	req, _ := http.NewRequest("GET", "/api/skills/coding", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "coding", response["name"])
	assert.Equal(t, "", response["instructions"])
}

func TestSkills_Get_NotFound(t *testing.T) {
	handler := setupSkillTestHandler(t, newMockSkillProvider())

	router := gin.New()
	router.GET("/api/skills/:name", handler.GetSkill)

	req, _ := http.NewRequest("GET", "/api/skills/nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response["error"], "not found")
}

func TestSkills_Enable(t *testing.T) {
	sp := newMockSkillProvider()
	handler := setupSkillTestHandler(t, sp)

	router := gin.New()
	router.POST("/api/skills/:name/enable", handler.EnableSkill)

	req, _ := http.NewRequest("POST", "/api/skills/review/enable", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "review", response["name"])
	assert.Equal(t, true, response["enabled"])

	assert.True(t, sp.enabledMap["review"])
}

func TestSkills_Disable(t *testing.T) {
	sp := newMockSkillProvider()
	handler := setupSkillTestHandler(t, sp)

	router := gin.New()
	router.POST("/api/skills/:name/disable", handler.DisableSkill)

	req, _ := http.NewRequest("POST", "/api/skills/coding/disable", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "coding", response["name"])
	assert.Equal(t, false, response["enabled"])

	assert.False(t, sp.enabledMap["coding"])
}