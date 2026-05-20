package session

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/copcon/server/internal/testutil"
)

const testDBName = "agent_infra_test"

func setupTestDB(t *testing.T) *gorm.DB {
	dsn := "host=localhost port=5432 user=admin password=changeme dbname=postgres sslmode=disable"

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", testDBName))
	db.Exec(fmt.Sprintf("CREATE DATABASE %s", testDBName))

	sqlDB, _ := db.DB()
	sqlDB.Close()

	testDSN := fmt.Sprintf("host=localhost port=5432 user=admin password=changeme dbname=%s sslmode=disable", testDBName)
	testDB, err := gorm.Open(postgres.Open(testDSN), &gorm.Config{})
	require.NoError(t, err)

	err = testDB.AutoMigrate(&Session{}, &Message{})
	require.NoError(t, err)

	t.Cleanup(func() {
		testDB.Exec("DROP TABLE IF EXISTS messages")
		testDB.Exec("DROP TABLE IF EXISTS sessions")
		sqlDB, _ := testDB.DB()
		sqlDB.Close()

		cleanupDB, _ := gorm.Open(postgres.Open(dsn), &gorm.Config{})
		cleanupDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", testDBName))
		sqlCleanup, _ := cleanupDB.DB()
		sqlCleanup.Close()
	})

	return testDB
}

func TestSessionManager_Create(t *testing.T) {
	db := setupTestDB(t)
	mgr := NewSessionManager(db, nil)
	ctx := context.Background()
	chatCtx := testutil.NewMockChatContext(ctx, "", "")

	session, err := mgr.Create(chatCtx, "Test Session", "")

	assert.NoError(t, err)
	assert.NotNil(t, session)
	assert.NotEqual(t, uuid.Nil, session.ID)
	assert.Equal(t, "Test Session", session.Title)
	assert.NotNil(t, session.Metadata)
}

func TestSessionManager_Get(t *testing.T) {
	db := setupTestDB(t)
	mgr := NewSessionManager(db, nil)
	ctx := context.Background()

	chatCtx := testutil.NewMockChatContext(ctx, "", "")
	created, err := mgr.Create(chatCtx, "Test Session", "")
	require.NoError(t, err)

	chatCtxForGet := testutil.NewMockChatContext(ctx, created.ID.String(), "")
	session, err := mgr.Get(chatCtxForGet)

	assert.NoError(t, err)
	assert.Equal(t, created.ID, session.ID)
	assert.Equal(t, "Test Session", session.Title)
}

func TestSessionManager_Get_NotFound(t *testing.T) {
	db := setupTestDB(t)
	mgr := NewSessionManager(db, nil)
	ctx := context.Background()

	chatCtx := testutil.NewMockChatContext(ctx, uuid.New().String(), "")
	_, err := mgr.Get(chatCtx)

	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestSessionManager_List(t *testing.T) {
	db := setupTestDB(t)
	mgr := NewSessionManager(db, nil)
	ctx := context.Background()

	chatCtx := testutil.NewMockChatContext(ctx, "", "")
	_, err := mgr.Create(chatCtx, "Session 1", "")
	require.NoError(t, err)
	_, err = mgr.Create(chatCtx, "Session 2", "")
	require.NoError(t, err)

	chatCtxForList := testutil.NewMockChatContext(ctx, "", "")
	sessions, total, err := mgr.List(chatCtxForList, 10, 0)

	assert.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, sessions, 2)
}

func TestSessionManager_Delete(t *testing.T) {
	db := setupTestDB(t)
	mgr := NewSessionManager(db, nil)
	ctx := context.Background()

	chatCtx := testutil.NewMockChatContext(ctx, "", "")
	created, err := mgr.Create(chatCtx, "Test Session", "")
	require.NoError(t, err)

	chatCtxForDelete := testutil.NewMockChatContext(ctx, created.ID.String(), "")
	err = mgr.Delete(chatCtxForDelete)
	assert.NoError(t, err)

	chatCtxForGet := testutil.NewMockChatContext(ctx, created.ID.String(), "")
	_, err = mgr.Get(chatCtxForGet)
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestSessionManager_UpdateTitle(t *testing.T) {
	db := setupTestDB(t)
	mgr := NewSessionManager(db, nil)
	ctx := context.Background()

	chatCtx := testutil.NewMockChatContext(ctx, "", "")
	created, err := mgr.Create(chatCtx, "Old Title", "")
	require.NoError(t, err)

	chatCtxForUpdate := testutil.NewMockChatContext(ctx, created.ID.String(), "")
	err = mgr.UpdateTitle(chatCtxForUpdate, "New Title")
	assert.NoError(t, err)

	chatCtxForGet := testutil.NewMockChatContext(ctx, created.ID.String(), "")
	session, err := mgr.Get(chatCtxForGet)
	require.NoError(t, err)
	assert.Equal(t, "New Title", session.Title)
}

func TestCreateSessionWithAgent(t *testing.T) {
	db := setupTestDB(t)
	mgr := NewSessionManager(db, nil)
	ctx := context.Background()

	agentID := "agent-123"
	chatCtx := testutil.NewMockChatContext(ctx, "", agentID)
	session, err := mgr.Create(chatCtx, "Session with Agent", agentID)

	require.NoError(t, err)
	assert.NotNil(t, session)
	assert.NotEqual(t, uuid.Nil, session.ID)
	assert.Equal(t, "Session with Agent", session.Title)
	assert.Equal(t, agentID, session.DefaultAgentID)

	chatCtxForGet := testutil.NewMockChatContext(ctx, session.ID.String(), "")
	retrieved, err := mgr.Get(chatCtxForGet)
	require.NoError(t, err)
	assert.Equal(t, agentID, retrieved.DefaultAgentID)
}

func TestParentSessionID(t *testing.T) {
	db := setupTestDB(t)
	mgr := NewSessionManager(db, nil)
	ctx := context.Background()

	chatCtx := testutil.NewMockChatContext(ctx, "", "")

	// Create parent session
	parent, err := mgr.Create(chatCtx, "Parent Session", "")
	require.NoError(t, err)
	require.NotNil(t, parent)

	// Create child session with parent reference
	child, err := mgr.Create(chatCtx, "Child Session", "", WithParentSessionID(parent.ID))
	require.NoError(t, err)
	require.NotNil(t, child)
	require.NotNil(t, child.ParentSessionID)
	assert.Equal(t, parent.ID, *child.ParentSessionID)

	// Verify persistence
	chatCtxForGet := testutil.NewMockChatContext(ctx, child.ID.String(), "")
	retrieved, err := mgr.Get(chatCtxForGet)
	require.NoError(t, err)
	require.NotNil(t, retrieved.ParentSessionID)
	assert.Equal(t, parent.ID, *retrieved.ParentSessionID)
}

func TestParentSessionID_FKConstraint(t *testing.T) {
	db := setupTestDB(t)

	// AutoMigrate does not create FK constraints for standalone *uuid.UUID fields,
	// so we add the constraint manually to test the database-level FK enforcement.
	db.Exec("ALTER TABLE sessions DROP CONSTRAINT IF EXISTS fk_sessions_parent")
	err := db.Exec("ALTER TABLE sessions ADD CONSTRAINT fk_sessions_parent FOREIGN KEY (parent_session_id) REFERENCES sessions(id)").Error
	require.NoError(t, err)

	mgr := NewSessionManager(db, nil)
	ctx := context.Background()

	chatCtx := testutil.NewMockChatContext(ctx, "", "")

	// Create parent + child
	parent, err := mgr.Create(chatCtx, "Parent", "")
	require.NoError(t, err)

	_, err = mgr.Create(chatCtx, "Child", "", WithParentSessionID(parent.ID))
	require.NoError(t, err)

	// Deleting parent should fail due to FK constraint from child
	chatCtxForDelete := testutil.NewMockChatContext(ctx, parent.ID.String(), "")
	err = mgr.Delete(chatCtxForDelete)
	assert.Error(t, err, "deleting parent session with children should violate FK constraint")
	assert.Contains(t, err.Error(), "violates foreign key constraint")
}
