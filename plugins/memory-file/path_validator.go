package memoryfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidatePath rejects paths that could escape the agent's memory directory.
// It rejects: ".." components, absolute paths, dotfile names, and paths
// that would resolve to a symlink (security-sensitive for file-based storage).
func ValidatePath(relPath string) error {
	if relPath == "" {
		return fmt.Errorf("path must not be empty")
	}

	if filepath.IsAbs(relPath) {
		return fmt.Errorf("absolute paths are not allowed: %s", relPath)
	}

	cleaned := filepath.Clean(relPath)
	if cleaned != relPath {
		return fmt.Errorf("path contains unclean components: %s", relPath)
	}

	parts := strings.Split(cleaned, string(filepath.Separator))
	for _, part := range parts {
		if part == ".." {
			return fmt.Errorf("path traversal not allowed: %s", relPath)
		}
		if part == "." {
			continue
		}
		if strings.HasPrefix(part, ".") {
			return fmt.Errorf("dotfiles are not allowed: %s", part)
		}
	}

	return nil
}

// IsSymlink checks if the path is or contains a symlink.
func IsSymlink(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to stat %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return true, nil
	}
	return false, nil
}
