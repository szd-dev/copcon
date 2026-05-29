package sqlitevec

import (
	"fmt"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/copcon/core/storage"
)

func init() {
	storage.RegisterKnowledgeStoreProvider("sqlite-vec", func(config map[string]any) (storage.KnowledgeStore, error) {
		dsn := ":memory:"
		if v, ok := config["dsn"].(string); ok && v != "" {
			dsn = v
		}

		db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
		if err != nil {
			return nil, fmt.Errorf("open sqlite database: %w", err)
		}

		ks, err := NewKnowledgeStore(db)
		if err != nil {
			return nil, fmt.Errorf("create knowledge store: %w", err)
		}
		return ks, nil
	})
}
