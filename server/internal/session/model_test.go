package session

import (
	"context"
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
