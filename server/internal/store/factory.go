package store

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	sqlitestore "github.com/copcon/plugins/storage-sqlite"
	pgstore "github.com/copcon/plugins/storage-postgres"
	"github.com/copcon/core/storage"
	"github.com/copcon/server/internal/config"
)

// CreateStoreProvider creates a storage.StoreProvider based on the database configuration.
// If cfg.Type is "sqlite" or (cfg.Type is "" and no PostgreSQL config is present),
// it uses SQLite. Otherwise it uses PostgreSQL.
func CreateStoreProvider(cfg config.DatabaseConfig) (storage.StoreProvider, error) {
	// Ambiguous: both host and sqlite_path set without explicit type
	if cfg.Type == "" && cfg.Host != "" && cfg.SQLitePath != "" {
		return nil, fmt.Errorf("ambiguous database config: both host (%s) and sqlite_path (%s) specified without explicit type",
			cfg.Host, cfg.SQLitePath)
	}

	useSQLite := cfg.Type == "sqlite" || (cfg.Type == "" && !cfg.HasPostgresConfig())

	if useSQLite {
		return createSQLiteProvider(cfg)
	}
	return createPostgresProvider(cfg)
}

func createSQLiteProvider(cfg config.DatabaseConfig) (storage.StoreProvider, error) {
	path := cfg.SQLitePath
	if path == "" {
		path = "data/copcon.db"
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create sqlite data directory %s: %w", dir, err)
	}

	// Build DSN with PRAGMA parameters for optimal SQLite behavior
	dsn := fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)", path)

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open sqlite database %s: %w", path, err)
	}

	// SQLite requires single-writer — limit to 1 open connection
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get underlying sqlite db: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	slog.Info("using SQLite", "path", path)
	return sqlitestore.NewStore(db), nil
}

func createPostgresProvider(cfg config.DatabaseConfig) (storage.StoreProvider, error) {
	db, err := gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open postgres database: %w", err)
	}
	slog.Info("using PostgreSQL", "host", cfg.Host)
	return pgstore.NewStore(db), nil
}
