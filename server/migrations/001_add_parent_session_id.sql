-- Migration 001: Add parent_session_id column to sessions table
-- Creates a self-referencing foreign key for session tree/hierarchy support.

ALTER TABLE sessions
    ADD COLUMN IF NOT EXISTS parent_session_id UUID REFERENCES sessions(id);

CREATE INDEX IF NOT EXISTS idx_sessions_parent_session_id ON sessions(parent_session_id);