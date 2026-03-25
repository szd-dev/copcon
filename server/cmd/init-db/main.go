package main

import (
	"fmt"
	"log"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	dsn := "host=localhost port=5432 user=admin password=changeme dbname=postgres sslmode=disable"

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}

	var exists bool
	db.Raw("SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = 'copcon')").Scan(&exists)

	if !exists {
		if err := db.Exec("CREATE DATABASE copcon").Error; err != nil {
			log.Fatalf("Failed to create database: %v", err)
		}
		fmt.Println("Database 'copcon' created")
	} else {
		fmt.Println("Database 'copcon' already exists")
	}

	sqlDB, _ := db.DB()
	sqlDB.Close()

	copconDSN := "host=localhost port=5432 user=admin password=changeme dbname=copcon sslmode=disable"
	copconDB, err := gorm.Open(postgres.Open(copconDSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to copcon: %v", err)
	}

	schema := `
CREATE TABLE IF NOT EXISTS sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title VARCHAR(255),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    metadata JSONB DEFAULT '{}'
);

CREATE TABLE IF NOT EXISTS messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    role VARCHAR(20) NOT NULL CHECK (role IN ('user', 'assistant', 'tool', 'system')),
    content TEXT NOT NULL,
    tool_calls JSONB,
    tool_call_id VARCHAR(255),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_messages_session_id ON messages(session_id);
CREATE INDEX IF NOT EXISTS idx_messages_created_at ON messages(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_updated_at ON sessions(updated_at DESC);

CREATE OR REPLACE FUNCTION update_session_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trigger_update_session_timestamp
    BEFORE UPDATE ON sessions
    FOR EACH ROW
    EXECUTE FUNCTION update_session_timestamp();
`

	if err := copconDB.Exec(schema).Error; err != nil {
		log.Fatalf("Failed to execute schema: %v", err)
	}

	fmt.Println("Tables created successfully!")
}
