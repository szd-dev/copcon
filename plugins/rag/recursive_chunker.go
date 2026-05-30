package rag

import (
	"strings"
)

type RecursiveChunker struct{}

func NewRecursiveChunker() *RecursiveChunker {
	return &RecursiveChunker{}
}

func splitSentences(text string) []string {
	var sentences []string
	start := 0
	for i := 0; i < len(text)-1; i++ {
		if (text[i] == '.' || text[i] == '!' || text[i] == '?') &&
			(text[i+1] == ' ' || text[i+1] == '\n' || text[i+1] == '\t') {
			sentences = append(sentences, strings.TrimSpace(text[start:i+1]))
			start = i + 1
			for start < len(text) && (text[start] == ' ' || text[start] == '\n' || text[start] == '\t') {
				start++
			}
		}
	}
	if start < len(text) {
		remaining := strings.TrimSpace(text[start:])
		if remaining != "" {
			sentences = append(sentences, remaining)
		}
	}
	return sentences
}

func (c *RecursiveChunker) Chunk(text string, opts ChunkOptions) ([]ChunkResult, error) {
	if text == "" {
		return nil, nil
	}

	chunkSize := opts.ChunkSize
	if chunkSize <= 0 {
		chunkSize = 1000
	}
	overlap := opts.ChunkOverlap
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= chunkSize {
		overlap = chunkSize / 4
	}

	paragraphs := splitParagraphs(text)
	merged := mergeParagraphs(paragraphs, chunkSize)
	return applyOverlap(merged, overlap), nil
}

func splitParagraphs(text string) []string {
	raw := strings.Split(text, "\n\n")
	var result []string
	for _, p := range raw {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func mergeParagraphs(paragraphs []string, chunkSize int) []string {
	var chunks []string
	var current strings.Builder

	for _, p := range paragraphs {
		if len(p) > chunkSize {
			if current.Len() > 0 {
				chunks = append(chunks, current.String())
				current.Reset()
			}
			subs := splitLargeParagraph(p, chunkSize)
			chunks = append(chunks, subs...)
			continue
		}

		if current.Len() > 0 && current.Len()+1+len(p) > chunkSize {
			chunks = append(chunks, current.String())
			current.Reset()
		}

		if current.Len() > 0 {
			current.WriteString("\n\n")
		}
		current.WriteString(p)
	}

	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}

	return chunks
}

func splitLargeParagraph(text string, chunkSize int) []string {
	sentences := splitSentences(text)
	var chunks []string
	var current strings.Builder

	for _, s := range sentences {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}

		if len(s) > chunkSize {
			if current.Len() > 0 {
				chunks = append(chunks, current.String())
				current.Reset()
			}
			hardChunks := hardSplit(s, chunkSize)
			chunks = append(chunks, hardChunks...)
			continue
		}

		if current.Len() > 0 && current.Len()+1+len(s) > chunkSize {
			chunks = append(chunks, current.String())
			current.Reset()
		}

		if current.Len() > 0 {
			current.WriteString(" ")
		}
		current.WriteString(s)
	}

	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}

	return chunks
}

func hardSplit(text string, chunkSize int) []string {
	var chunks []string
	for len(text) > chunkSize {
		boundary := findWordBoundary(text, chunkSize)
		chunks = append(chunks, strings.TrimSpace(text[:boundary]))
		text = text[boundary:]
	}
	if text != "" {
		chunks = append(chunks, strings.TrimSpace(text))
	}
	return chunks
}

func findWordBoundary(text string, pos int) int {
	if pos >= len(text) {
		return len(text)
	}
	for i := pos; i > pos/2; i-- {
		if text[i] == ' ' || text[i] == '\n' {
			return i
		}
	}
	return pos
}

func applyOverlap(chunks []string, overlap int) []ChunkResult {
	if len(chunks) == 0 {
		return nil
	}

	results := make([]ChunkResult, len(chunks))
	for i, chunk := range chunks {
		if overlap > 0 && i > 0 {
			prev := chunks[i-1]
			start := len(prev) - overlap
			if start < 0 {
				start = 0
			}
			chunk = prev[start:] + "\n" + chunk
		}
		results[i] = ChunkResult{
			Content: chunk,
			Index:   i,
		}
	}
	return results
}
