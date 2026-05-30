package memoryfile

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	defaultMaxIndexLines = 200
	defaultMaxIndexBytes = 25 * 1024
	indexFileName        = "INDEX.md"
)

// IndexEntry represents a single entry in the memory INDEX.md file.
type IndexEntry struct {
	RelPath   string
	Name      string
	Category  string
	UpdatedAt string
}

// BuildIndex scans the knowledge/ and archive/ directories for an agent
// and generates an INDEX.md file. The index is truncated to enforce
// maxLines and maxBytes limits.
func BuildIndex(basePath, agentID string, maxLines, maxIndexBytes int) error {
	agentDir := AgentDir(basePath, agentID)
	entries, err := collectEntries(agentDir)
	if err != nil {
		return fmt.Errorf("failed to collect entries: %w", err)
	}

	content := formatIndex(entries, maxLines, maxIndexBytes)
	indexDir := filepath.Join(agentDir, "system")
	return WriteFileWithPerms(filepath.Join(indexDir, indexFileName), []byte(content))
}

// ReadIndex reads the INDEX.md content for an agent.
func ReadIndex(basePath, agentID string) (string, error) {
	indexPath := filepath.Join(basePath, agentID, "system", indexFileName)
	data, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read index: %w", err)
	}
	return string(data), nil
}

// AddToIndex adds an entry to the existing INDEX.md by regenerating it.
func AddToIndex(basePath, agentID string, entry IndexEntry) error {
	return BuildIndex(basePath, agentID, defaultMaxIndexLines, defaultMaxIndexBytes)
}

// RemoveFromIndex removes an entry from INDEX.md by regenerating it.
func RemoveFromIndex(basePath, agentID, relPath string) error {
	return BuildIndex(basePath, agentID, defaultMaxIndexLines, defaultMaxIndexBytes)
}

func collectEntries(agentDir string) ([]IndexEntry, error) {
	var entries []IndexEntry

	for _, subdir := range []string{"knowledge", "archive"} {
		dir := filepath.Join(agentDir, subdir)
		files, err := scanMDFiles(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, f := range files {
			fullPath := filepath.Join(dir, f)
			fm, _, err := readFileFrontmatter(fullPath)
			if err != nil {
				continue
			}
			relPath := filepath.Join(subdir, f)
			entries = append(entries, IndexEntry{
				RelPath:   relPath,
				Name:      fm.Name,
				Category:  fm.Category,
				UpdatedAt: fm.UpdatedAt.Format("2006-01-02"),
			})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].RelPath < entries[j].RelPath
	})

	return entries, nil
}

func scanMDFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".md") || name == indexFileName {
			continue
		}
		files = append(files, name)
	}
	return files, nil
}

func readFileFrontmatter(path string) (Frontmatter, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Frontmatter{}, "", err
	}
	return ParseFrontmatter(data)
}

func formatIndex(entries []IndexEntry, maxLines, maxIndexBytes int) string {
	var buf strings.Builder
	buf.WriteString("# Memory Index\n\n")

	if len(entries) == 0 {
		buf.WriteString("No memory files.\n")
		return truncateIndex(buf.String(), maxLines, maxIndexBytes)
	}

	for _, e := range entries {
		line := fmt.Sprintf("- **%s** (`%s`) — %s [%s]\n", e.Name, e.RelPath, e.Category, e.UpdatedAt)
		buf.WriteString(line)
	}

	return truncateIndex(buf.String(), maxLines, maxIndexBytes)
}

func truncateIndex(content string, maxLines, maxBytes int) string {
	if maxLines <= 0 {
		maxLines = defaultMaxIndexLines
	}
	if maxBytes <= 0 {
		maxBytes = defaultMaxIndexBytes
	}

	lines := strings.Split(content, "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		lines = append(lines, "", fmt.Sprintf("... truncated (%d entries omitted)", len(lines)-maxLines))
	}

	result := strings.Join(lines, "\n")
	if len(result) > maxBytes {
		result = result[:maxBytes]
		// Find last complete line
		if idx := bytes.LastIndex([]byte(result), []byte("\n")); idx > 0 {
			result = result[:idx]
		}
		result += "\n\n... truncated (size limit reached)"
	}

	return result
}
