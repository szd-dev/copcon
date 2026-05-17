package session

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionWithDefaultAgentID(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Test 1: Session should have DefaultAgentID field
	session := &Session{
		Title:          "Test Session",
		DefaultAgentID: "agent-123",
	}

	err := db.WithContext(ctx).Create(session).Error
	require.NoError(t, err)

	// Verify the field is persisted
	var retrieved Session
	err = db.WithContext(ctx).First(&retrieved, "id = ?", session.ID).Error
	require.NoError(t, err)

	assert.Equal(t, "agent-123", retrieved.DefaultAgentID)
	assert.NotEqual(t, uuid.Nil, retrieved.ID)
}

func TestSessionDefaultAgentID_Empty(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Test 2: Session can have empty DefaultAgentID
	session := &Session{
		Title: "Test Session Without Agent",
	}

	err := db.WithContext(ctx).Create(session).Error
	require.NoError(t, err)

	var retrieved Session
	err = db.WithContext(ctx).First(&retrieved, "id = ?", session.ID).Error
	require.NoError(t, err)

	assert.Empty(t, retrieved.DefaultAgentID)
}

func TestSessionDefaultAgentID_MaxLength(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Test 3: DefaultAgentID respects size:64 constraint
	// PostgreSQL varchar(64) will reject values longer than 64 characters
	longAgentID := "this-is-a-very-long-agent-id-that-exceeds-sixty-four-characters-limit"
	session := &Session{
		Title:          "Test Session",
		DefaultAgentID: longAgentID,
	}

	err := db.WithContext(ctx).Create(session).Error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "value too long")

	// Verify that exactly 64 characters works
	exact64Chars := "this-is-exactly-sixty-four-characters-long-agent-id-1234567890ab"
	session2 := &Session{
		Title:          "Test Session 2",
		DefaultAgentID: exact64Chars,
	}

	err = db.WithContext(ctx).Create(session2).Error
	require.NoError(t, err)

	var retrieved Session
	err = db.WithContext(ctx).First(&retrieved, "id = ?", session2.ID).Error
	require.NoError(t, err)
	assert.Equal(t, exact64Chars, retrieved.DefaultAgentID)
}

func TestPersistedParts_ScanCamelCase(t *testing.T) {
	input := `[{"type":"tool-call","text":"hello","state":"running","toolCallId":"call_123","toolName":"bash","args":"{\"cmd\":\"ls\"}","output":"file.txt","error":"","stepIndex":1}]`

	var parts PersistedParts
	err := parts.Scan([]byte(input))
	require.NoError(t, err)
	require.Len(t, parts, 1)

	p := parts[0]
	assert.Equal(t, "tool-call", p.Type)
	assert.Equal(t, "hello", p.Text)
	assert.Equal(t, "running", p.State)
	assert.Equal(t, "call_123", p.ToolCallID)
	assert.Equal(t, "bash", p.ToolName)
	assert.Equal(t, `{"cmd":"ls"}`, p.Args)
	assert.Equal(t, "file.txt", p.Output)
	assert.Equal(t, "", p.Error)
	assert.Equal(t, 1, p.StepIndex)
}

func TestPersistedParts_ScanSnakeCase(t *testing.T) {
	input := `[{"type":"tool-call","tool_call_id":"call_abc","tool_name":"python","args":"print(1)","state":"done"}]`

	var parts PersistedParts
	err := parts.Scan([]byte(input))
	require.NoError(t, err)
	require.Len(t, parts, 1)

	p := parts[0]
	assert.Equal(t, "call_abc", p.ToolCallID)
	assert.Equal(t, "python", p.ToolName)
	assert.Equal(t, 0, p.StepIndex)
}

func TestPersistedParts_ScanMissingStepIndex(t *testing.T) {
	input := `[{"type":"text","text":"hi","state":"done"}]`

	var parts PersistedParts
	err := parts.Scan([]byte(input))
	require.NoError(t, err)
	require.Len(t, parts, 1)

	assert.Equal(t, 0, parts[0].StepIndex)
}

func TestPersistedParts_ScanCamelCaseOverridesSnakeCase(t *testing.T) {
	input := `[{"type":"tool-call","toolCallId":"camel","tool_call_id":"snake","toolName":"camelName","tool_name":"snakeName","state":"done"}]`

	var parts PersistedParts
	err := parts.Scan([]byte(input))
	require.NoError(t, err)
	require.Len(t, parts, 1)

	assert.Equal(t, "camel", parts[0].ToolCallID)
	assert.Equal(t, "camelName", parts[0].ToolName)
}

func TestPersistedParts_ScanEdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		input  interface{}
		expect PersistedParts
	}{
		{"nil", nil, nil},
		{"empty bytes", []byte{}, nil},
		{"null json", []byte("null"), nil},
		{"object json", []byte(`{"type":"text"}`), nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var parts PersistedParts
			err := parts.Scan(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expect, parts)
		})
	}
}

func TestPersistedParts_ValueCamelCase(t *testing.T) {
	parts := PersistedParts{
		{Type: "text", Text: "hello", State: "done"},
		{Type: "tool-call", ToolCallID: "call_1", ToolName: "bash", State: "pending", StepIndex: 2},
	}

	val, err := parts.Value()
	require.NoError(t, err)

	var raw []map[string]any
	err = json.Unmarshal(val.([]byte), &raw)
	require.NoError(t, err)

	require.Len(t, raw, 2)
	assert.Equal(t, "call_1", raw[1]["toolCallId"])
	assert.Equal(t, "bash", raw[1]["toolName"])
	assert.Equal(t, float64(2), raw[1]["stepIndex"])

	_, hasSnakeID := raw[1]["tool_call_id"]
	assert.False(t, hasSnakeID, "Value should produce camelCase keys, not snake_case")
}

func TestPersistedParts_ValueNil(t *testing.T) {
	var parts PersistedParts
	val, err := parts.Value()
	require.NoError(t, err)
	assert.Nil(t, val)
}

func TestPersistedParts_GormDataType(t *testing.T) {
	var parts PersistedParts
	assert.Equal(t, "jsonb", parts.GormDataType())
}

func TestPersistedParts_ScanLegacyTypeTextDelta(t *testing.T) {
	input := `[{"type":"text_delta","text_delta":"Hello world","state":"done"}]`

	var parts PersistedParts
	err := parts.Scan([]byte(input))
	require.NoError(t, err)
	require.Len(t, parts, 1)

	assert.Equal(t, "text", parts[0].Type, "text_delta type should be normalized to text")
	assert.Equal(t, "Hello world", parts[0].Text, "text_delta key should map to Text field")
	assert.Equal(t, 0, parts[0].StepIndex)
}

func TestPersistedParts_ScanLegacyTypeToolCall(t *testing.T) {
	input := `[{"type":"tool_call","tool_call_id":"call_123","tool_name":"read_file","state":"done"}]`

	var parts PersistedParts
	err := parts.Scan([]byte(input))
	require.NoError(t, err)
	require.Len(t, parts, 1)

	assert.Equal(t, "tool-call", parts[0].Type, "tool_call type should be normalized to tool-call")
	assert.Equal(t, "call_123", parts[0].ToolCallID)
	assert.Equal(t, "read_file", parts[0].ToolName)
	assert.Equal(t, 0, parts[0].StepIndex)
}

func TestPersistedParts_ScanStepIndexSnakeCase(t *testing.T) {
	input := `[{"type":"text","text":"hi","state":"done","step_index":3}]`

	var parts PersistedParts
	err := parts.Scan([]byte(input))
	require.NoError(t, err)
	require.Len(t, parts, 1)

	assert.Equal(t, 3, parts[0].StepIndex, "step_index snake_case key should be read")
}

func TestPersistedParts_ScanStepIndexCamelCaseOverridesSnakeCase(t *testing.T) {
	input := `[{"type":"text","text":"hi","state":"done","stepIndex":2,"step_index":5}]`

	var parts PersistedParts
	err := parts.Scan([]byte(input))
	require.NoError(t, err)
	require.Len(t, parts, 1)

	assert.Equal(t, 2, parts[0].StepIndex, "camelCase stepIndex should take precedence over snake_case step_index")
}

func TestPersistedParts_ScanTextDeltaKeyFallback(t *testing.T) {
	input := `[{"type":"text","text_delta":"fallback content","state":"done"}]`

	var parts PersistedParts
	err := parts.Scan([]byte(input))
	require.NoError(t, err)
	require.Len(t, parts, 1)

	assert.Equal(t, "text", parts[0].Type)
	assert.Equal(t, "fallback content", parts[0].Text, "text_delta key should be used when text key is absent")
}

func TestPersistedParts_ScanTextKeyOverridesTextDelta(t *testing.T) {
	input := `[{"type":"text","text":"primary","text_delta":"secondary","state":"done"}]`

	var parts PersistedParts
	err := parts.Scan([]byte(input))
	require.NoError(t, err)
	require.Len(t, parts, 1)

	assert.Equal(t, "primary", parts[0].Text, "text key should take precedence over text_delta")
}

func TestPersistedParts_ScanMixedLegacyAndNew(t *testing.T) {
	input := `[
		{"type":"text_delta","text_delta":"Hello","state":"done"},
		{"type":"tool_call","tool_call_id":"call_1","tool_name":"bash","state":"done","step_index":0},
		{"type":"text","text":"New format","state":"done","stepIndex":1},
		{"type":"tool-call","toolCallId":"call_2","toolName":"python","state":"done","stepIndex":1}
	]`

	var parts PersistedParts
	err := parts.Scan([]byte(input))
	require.NoError(t, err)
	require.Len(t, parts, 4)

	assert.Equal(t, "text", parts[0].Type)
	assert.Equal(t, "Hello", parts[0].Text)
	assert.Equal(t, 0, parts[0].StepIndex)

	assert.Equal(t, "tool-call", parts[1].Type)
	assert.Equal(t, "call_1", parts[1].ToolCallID)
	assert.Equal(t, "bash", parts[1].ToolName)
	assert.Equal(t, 0, parts[1].StepIndex)

	assert.Equal(t, "text", parts[2].Type)
	assert.Equal(t, "New format", parts[2].Text)
	assert.Equal(t, 1, parts[2].StepIndex)

	assert.Equal(t, "tool-call", parts[3].Type)
	assert.Equal(t, "call_2", parts[3].ToolCallID)
	assert.Equal(t, "python", parts[3].ToolName)
	assert.Equal(t, 1, parts[3].StepIndex)
}

func TestNormalizePartType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"text", "text"},
		{"text_delta", "text"},
		{"tool-call", "tool-call"},
		{"tool_call", "tool-call"},
		{"reasoning", "reasoning"},
		{"step-start", "step-start"},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, normalizePartType(tt.input))
		})
	}
}
