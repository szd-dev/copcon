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
	mgr := NewSessionManager(db)
	ctx := context.Background()

	session, err := mgr.Create(ctx, "Test Session")

	assert.NoError(t, err)
	assert.NotNil(t, session)
	assert.NotEqual(t, uuid.Nil, session.ID)
	assert.Equal(t, "Test Session", session.Title)
	assert.NotNil(t, session.Metadata)
}

func TestSessionManager_Get(t *testing.T) {
	db := setupTestDB(t)
	mgr := NewSessionManager(db)
	ctx := context.Background()

	created, err := mgr.Create(ctx, "Test Session")
	require.NoError(t, err)

	session, err := mgr.Get(ctx, created.ID.String())

	assert.NoError(t, err)
	assert.Equal(t, created.ID, session.ID)
	assert.Equal(t, "Test Session", session.Title)
}

func TestSessionManager_Get_NotFound(t *testing.T) {
	db := setupTestDB(t)
	mgr := NewSessionManager(db)
	ctx := context.Background()

	_, err := mgr.Get(ctx, uuid.New().String())

	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestSessionManager_List(t *testing.T) {
	db := setupTestDB(t)
	mgr := NewSessionManager(db)
	ctx := context.Background()

	_, err := mgr.Create(ctx, "Session 1")
	require.NoError(t, err)
	_, err = mgr.Create(ctx, "Session 2")
	require.NoError(t, err)

	sessions, total, err := mgr.List(ctx, 10, 0)

	assert.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, sessions, 2)
}

func TestSessionManager_Delete(t *testing.T) {
	db := setupTestDB(t)
	mgr := NewSessionManager(db)
	ctx := context.Background()

	created, err := mgr.Create(ctx, "Test Session")
	require.NoError(t, err)

	err = mgr.Delete(ctx, created.ID.String())
	assert.NoError(t, err)

	_, err = mgr.Get(ctx, created.ID.String())
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestSessionManager_UpdateTitle(t *testing.T) {
	db := setupTestDB(t)
	mgr := NewSessionManager(db)
	ctx := context.Background()

	created, err := mgr.Create(ctx, "Old Title")
	require.NoError(t, err)

	err = mgr.UpdateTitle(ctx, created.ID.String(), "New Title")
	assert.NoError(t, err)

	session, err := mgr.Get(ctx, created.ID.String())
	require.NoError(t, err)
	assert.Equal(t, "New Title", session.Title)
}
