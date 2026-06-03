package memoryfile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	factsFileName = "FACTS.md"
)

// FactEntry represents a single entry in the FACTS.md file,
// grouped by session ID.
type FactEntry struct {
	SessionID  string
	MessageIDs []string
	Name       string
	RelPath    string
}

// BuildFacts scans the knowledge/ and archive/ directories for an agent,
// extracts session_id from frontmatter, groups by session, and writes FACTS.md.
func BuildFacts(basePath, agentID string) error {
	agentDir := AgentDir(basePath, agentID)
	entries, err := collectFacts(agentDir)
	if err != nil {
		return fmt.Errorf("failed to collect facts: %w", err)
	}

	content := formatFacts(entries)
	factsDir := filepath.Join(agentDir, "system")
	return WriteFileWithPerms(filepath.Join(factsDir, factsFileName), []byte(content))
}

// ReadFacts reads the FACTS.md content for an agent.
func ReadFacts(basePath, agentID string) (string, error) {
	factsPath := filepath.Join(basePath, agentID, "system", factsFileName)
	data, err := os.ReadFile(factsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read facts: %w", err)
	}
	return string(data), nil
}

// AddToFacts adds an entry to FACTS.md by regenerating it (full rebuild).
func AddToFacts(basePath, agentID string) error {
	return BuildFacts(basePath, agentID)
}

// RemoveFromFacts removes an entry from FACTS.md by regenerating it (full rebuild).
func RemoveFromFacts(basePath, agentID string) error {
	return BuildFacts(basePath, agentID)
}

// collectFacts scans knowledge/ and archive/ for .md files, parses frontmatter,
// and returns FactEntry values for files that have a non-empty SessionID.
func collectFacts(agentDir string) ([]FactEntry, error) {
	var entries []FactEntry

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
			if fm.SessionID == "" {
				continue
			}
			relPath := filepath.Join(subdir, f)
			entries = append(entries, FactEntry{
				SessionID:  fm.SessionID,
				MessageIDs: fm.MessageIDs,
				Name:       fm.Name,
				RelPath:    relPath,
			})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].SessionID != entries[j].SessionID {
			return entries[i].SessionID < entries[j].SessionID
		}
		return entries[i].RelPath < entries[j].RelPath
	})

	return entries, nil
}

// formatFacts groups FactEntry values by SessionID and formats them as markdown.
func formatFacts(entries []FactEntry) string {
	if len(entries) == 0 {
		return "# Fact Index\n\nNo fact entries.\n"
	}

	var buf strings.Builder
	buf.WriteString("# Fact Index\n\n")

	currentSession := ""
	for _, e := range entries {
		if e.SessionID != currentSession {
			currentSession = e.SessionID
			buf.WriteString(fmt.Sprintf("## session: %s\n\n", currentSession))
		}

		if len(e.MessageIDs) == 0 {
			buf.WriteString(fmt.Sprintf("| - | [%s](%s) |\n", e.Name, e.RelPath))
		} else {
			for _, msgID := range e.MessageIDs {
				buf.WriteString(fmt.Sprintf("| %s | [%s](%s) |\n", msgID, e.Name, e.RelPath))
			}
		}
	}

	return buf.String()
}
