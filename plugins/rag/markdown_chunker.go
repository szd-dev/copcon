package rag

import (
	"regexp"
	"strings"
)

type MarkdownAwareChunker struct {
	fallback *RecursiveChunker
}

func NewMarkdownAwareChunker() *MarkdownAwareChunker {
	return &MarkdownAwareChunker{
		fallback: NewRecursiveChunker(),
	}
}

var headingRe = regexp.MustCompile(`(?m)^(#{1,6})\s+(.+)$`)

func (c *MarkdownAwareChunker) Chunk(text string, opts ChunkOptions) ([]ChunkResult, error) {
	if text == "" {
		return nil, nil
	}

	sections := splitByHeadings(text)
	if len(sections) == 0 {
		return c.fallback.Chunk(text, opts)
	}

	chunkSize := opts.ChunkSize
	if chunkSize <= 0 {
		chunkSize = 1000
	}

	var results []ChunkResult
	idx := 0

	for _, section := range sections {
		if len(section.Content) <= chunkSize {
			results = append(results, ChunkResult{
				Content:  section.Content,
				Index:    idx,
				Metadata: map[string]any{"heading_path": section.HeadingPath},
			})
			idx++
			continue
		}

		prefixedContent := section.Content
		if section.HeadingPath != "" {
			prefixedContent = section.HeadingPath + "\n\n" + prefixedContent
		}

		subChunks, err := c.fallback.Chunk(prefixedContent, opts)
		if err != nil {
			return nil, err
		}
		for _, sc := range subChunks {
			sc.Index = idx
			sc.Metadata = map[string]any{"heading_path": section.HeadingPath}
			results = append(results, sc)
			idx++
		}
	}

	return results, nil
}

type mdSection struct {
	HeadingPath string
	Content     string
}

func splitByHeadings(text string) []mdSection {
	lines := strings.Split(text, "\n")
	var sections []mdSection
	var contentLines []string
	currentLevel := 0
	pathStack := make([]string, 0, 6)

	flush := func() {
		if len(contentLines) > 0 {
			content := strings.TrimSpace(strings.Join(contentLines, "\n"))
			if content != "" {
				sections = append(sections, mdSection{
					HeadingPath: strings.Join(pathStack[:currentLevel], " > "),
					Content:     content,
				})
			}
		}
		contentLines = nil
	}

	for _, line := range lines {
		matches := headingRe.FindStringSubmatch(line)
		if matches != nil {
			flush()
			level := len(matches[1])
			title := matches[2]

			if level <= len(pathStack) {
				pathStack = pathStack[:level-1]
			}
			for len(pathStack) < level-1 {
				pathStack = append(pathStack, "")
			}
			pathStack = append(pathStack, title)

			currentLevel = level
			contentLines = []string{line}
			continue
		}

		contentLines = append(contentLines, line)
	}

	flush()
	return sections
}
