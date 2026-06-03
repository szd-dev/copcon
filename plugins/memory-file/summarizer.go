package memoryfile

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/copcon/core/llm"
)

// SummarizerConfig controls when and how the FileSummarizer triggers and runs.
type SummarizerConfig struct {
	MaxMemories     int // trigger when file count > this (default: 50)
	MaxAgeHours     int // trigger when oldest file > this (default: 24)
	CooldownMinutes int // minimum time between summaries (default: 60)
	Model           string
}

// DefaultSummarizerConfig returns a SummarizerConfig with sensible defaults.
func DefaultSummarizerConfig() SummarizerConfig {
	return SummarizerConfig{
		MaxMemories:     50,
		MaxAgeHours:     24,
		CooldownMinutes: 60,
	}
}

// FileSummarizer generates summaries of memory files with cooldown persistence.
type FileSummarizer struct {
	store  *FileMemoryStore
	llm    llm.LLMProvider
	config SummarizerConfig
	logger *slog.Logger
}

// NewFileSummarizer creates a new FileSummarizer. Zero or negative config
// values are replaced with defaults.
func NewFileSummarizer(store *FileMemoryStore, llmProvider llm.LLMProvider, config SummarizerConfig) *FileSummarizer {
	if config.MaxMemories <= 0 {
		config.MaxMemories = 50
	}
	if config.MaxAgeHours <= 0 {
		config.MaxAgeHours = 24
	}
	if config.CooldownMinutes <= 0 {
		config.CooldownMinutes = 60
	}
	return &FileSummarizer{
		store:  store,
		llm:    llmProvider,
		config: config,
		logger: slog.Default(),
	}
}

// ShouldTrigger returns true when a summary should be generated for the given
// agent. A summary is triggered when knowledge/ file count exceeds
// MaxMemories OR the oldest file is older than MaxAgeHours, AND the cooldown
// period has elapsed since the last summary.
func (s *FileSummarizer) ShouldTrigger(agentID string) bool {
	if s.llm == nil {
		return false
	}

	// Check cooldown
	if !s.isCooldownExpired(agentID) {
		return false
	}

	agentDir := AgentDir(s.store.BasePath(), agentID)
	knowledgeDir := filepath.Join(agentDir, "knowledge")

	files, err := scanMDFiles(knowledgeDir)
	if err != nil {
		return false
	}

	// Trigger by count
	if len(files) > s.config.MaxMemories {
		return true
	}

	// Trigger by age: check oldest file modification time.
	// scanMDFiles returns entries from os.ReadDir which are sorted by name,
	// so we check each file and return on the first that exceeds the age.
	for _, f := range files {
		fullPath := filepath.Join(knowledgeDir, f)
		info, err := os.Stat(fullPath)
		if err != nil {
			continue
		}
		age := time.Since(info.ModTime())
		if age.Hours() > float64(s.config.MaxAgeHours) {
			return true
		}
		break // only need to check the first stat-able file for oldest
	}

	return false
}

// isCooldownExpired reads the .last_summary timestamp and returns true if the
// cooldown period has elapsed (or if no previous summary exists).
func (s *FileSummarizer) isCooldownExpired(agentID string) bool {
	lastSummaryPath := filepath.Join(s.store.BasePath(), agentID, "system", ".last_summary")
	data, err := os.ReadFile(lastSummaryPath)
	if err != nil {
		return true // no file = never summarized = cooldown expired
	}

	lastTime, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
	if err != nil {
		return true
	}

	return time.Since(lastTime).Minutes() >= float64(s.config.CooldownMinutes)
}

// recordSummaryTime persists the current timestamp as the last summary time.
func (s *FileSummarizer) recordSummaryTime(agentID string) {
	lastSummaryPath := filepath.Join(s.store.BasePath(), agentID, "system", ".last_summary")
	now := time.Now().Format(time.RFC3339)
	_ = WriteFileWithPerms(lastSummaryPath, []byte(now))
}

// Summarize collects all memory files from knowledge/ and archive/, sends them
// to the LLM for summarization, and writes the result as a system summary file.
func (s *FileSummarizer) Summarize(ctx context.Context, agentID string) error {
	agentDir := AgentDir(s.store.BasePath(), agentID)

	// Collect all memory files from knowledge/ and archive/
	var allContent strings.Builder
	var sourceFiles []string

	for _, subdir := range []string{"knowledge", "archive"} {
		dir := filepath.Join(agentDir, subdir)
		files, err := scanMDFiles(dir)
		if err != nil {
			continue
		}
		for _, f := range files {
			fullPath := filepath.Join(dir, f)
			data, err := os.ReadFile(fullPath)
			if err != nil {
				continue
			}
			relPath := filepath.Join(subdir, f)
			sourceFiles = append(sourceFiles, relPath)
			fmt.Fprintf(&allContent, "## %s\n\n%s\n\n", relPath, string(data))
		}
	}

	if allContent.Len() == 0 {
		return nil
	}

	prompt := s.buildSummaryPrompt(allContent.String())

	summary, err := Complete(ctx, s.llm, llm.StreamParams{
		Model: s.config.Model,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: prompt},
		},
		Temperature: 0.3,
	})
	if err != nil {
		return fmt.Errorf("LLM summary call failed: %w", err)
	}

	// Write summary file
	date := time.Now().Format("2006-01-02")
	summaryName := fmt.Sprintf("summary-%s", date)

	fm := Frontmatter{
		Name:        summaryName,
		Category:    "system",
		SourceFiles: sourceFiles,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	data := SerializeFrontmatter(fm, summary)
	summaryPath := filepath.Join(agentDir, "system", summaryName+".md")

	if err := WriteFileWithPerms(summaryPath, data); err != nil {
		return fmt.Errorf("failed to write summary: %w", err)
	}

	// Record cooldown
	s.recordSummaryTime(agentID)

	// Clean old summaries — keep last 3
	s.cleanupOldSummaries(agentID)

	return nil
}

// cleanupOldSummaries removes the oldest summary files when more than 3 exist.
func (s *FileSummarizer) cleanupOldSummaries(agentID string) {
	systemDir := filepath.Join(s.store.BasePath(), agentID, "system")
	entries, err := os.ReadDir(systemDir)
	if err != nil {
		return
	}

	var summaryFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "summary-") && strings.HasSuffix(e.Name(), ".md") {
			summaryFiles = append(summaryFiles, e.Name())
		}
	}

	sort.Strings(summaryFiles) // alphabetical = chronological for date-based names

	for len(summaryFiles) > 3 {
		oldest := summaryFiles[0]
		_ = os.Remove(filepath.Join(systemDir, oldest))
		summaryFiles = summaryFiles[1:]
	}
}

// buildSummaryPrompt constructs the LLM prompt for summarization.
func (s *FileSummarizer) buildSummaryPrompt(content string) string {
	return fmt.Sprintf(`You are a memory summarization assistant. Your task is to condense the following memory files into a concise, well-organized summary that preserves all important facts, decisions, and context.

Rules:
- Preserve specific facts, numbers, names, and dates
- Group related information thematically
- Remove redundancy while keeping unique details
- Use clear headings and bullet points
- Do not add information that isn't present in the source files

Memory files to summarize:

%s`, content)
}
