package memoryfile

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	dirPerm  os.FileMode = 0o700
	filePerm os.FileMode = 0o600
)

// EnsureAgentDirs creates the system/knowledge/archive subdirectories for an agent.
func EnsureAgentDirs(basePath, agentID string) error {
	agentDir := filepath.Join(basePath, agentID)
	for _, sub := range []string{"system", "knowledge", "archive"} {
		dir := filepath.Join(agentDir, sub)
		if err := os.MkdirAll(dir, dirPerm); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	return nil
}

// AgentDir returns the root directory for an agent's memory files.
func AgentDir(basePath, agentID string) string {
	return filepath.Join(basePath, agentID)
}

// WriteFileWithPerms writes data to a file with 0o600 permissions.
func WriteFileWithPerms(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}
	if err := os.WriteFile(path, data, filePerm); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	return nil
}
