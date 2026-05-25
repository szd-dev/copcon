package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/copcon/server/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateStoreProvider_AutoSQLite(t *testing.T) {
	// Empty config should auto-select SQLite and create data/ directory
	cfg := config.DatabaseConfig{}
	provider, err := CreateStoreProvider(cfg)
	require.NoError(t, err)
	assert.NotNil(t, provider.Sessions())

	// Verify the default data directory was created
	_, err = os.Stat("data")
	assert.NoError(t, err, "default data directory should exist")

	// Cleanup
	os.RemoveAll("data")
}

func TestCreateStoreProvider_ExplicitSQLite(t *testing.T) {
	// Explicit type=sqlite should use SQLite regardless of other fields
	cfg := config.DatabaseConfig{Type: "sqlite"}
	provider, err := CreateStoreProvider(cfg)
	require.NoError(t, err)
	assert.NotNil(t, provider.Sessions())

	// Cleanup default data directory
	os.RemoveAll("data")
}

func TestCreateStoreProvider_PostgresUnavailable(t *testing.T) {
	// Full PG config should attempt Postgres; skip if unavailable
	cfg := config.DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "admin",
		Password: "changeme",
		DBName:   "copcon",
	}
	_, err := CreateStoreProvider(cfg)
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
}

func TestCreateStoreProvider_PostgresMissingHost(t *testing.T) {
	// type=postgres with empty host should fail with error containing "postgres"
	cfg := config.DatabaseConfig{Type: "postgres"}
	_, err := CreateStoreProvider(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "postgres")
}

func TestCreateStoreProvider_AmbiguousConfig(t *testing.T) {
	// Both host and sqlite_path set without explicit type should error
	cfg := config.DatabaseConfig{
		Host:       "localhost",
		SQLitePath: "/tmp/test.db",
	}
	_, err := CreateStoreProvider(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ambiguous")
}

func TestCreateStoreProvider_CustomSQLitePath(t *testing.T) {
	// Custom SQLitePath should create the file at that location
	path := filepath.Join(t.TempDir(), "custom.db")
	cfg := config.DatabaseConfig{SQLitePath: path}
	provider, err := CreateStoreProvider(cfg)
	require.NoError(t, err)
	assert.NotNil(t, provider.Sessions())

	// Verify file exists at custom path
	_, err = os.Stat(path)
	assert.NoError(t, err, "custom database file should exist at specified path")
}