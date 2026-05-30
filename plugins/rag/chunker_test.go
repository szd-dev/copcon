package rag

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecursiveChunkerBasic(t *testing.T) {
	c := NewRecursiveChunker()
	text := "Paragraph one.\n\nParagraph two.\n\nParagraph three."
	results, err := c.Chunk(text, ChunkOptions{ChunkSize: 500, ChunkOverlap: 0})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Contains(t, results[0].Content, "Paragraph one")
}

func TestRecursiveChunkerLargeParagraph(t *testing.T) {
	c := NewRecursiveChunker()
	words := make([]string, 200)
	for i := range words {
		words[i] = "word"
	}
	text := strings.Join(words, " ")

	results, err := c.Chunk(text, ChunkOptions{ChunkSize: 100, ChunkOverlap: 0})
	require.NoError(t, err)
	assert.Greater(t, len(results), 1)
	for _, r := range results {
		assert.NotEmpty(t, r.Content)
	}
}

func TestRecursiveChunkerOverlap(t *testing.T) {
	c := NewRecursiveChunker()
	p1 := strings.Repeat("a", 50)
	p2 := strings.Repeat("b", 50)
	p3 := strings.Repeat("c", 50)
	text := p1 + "\n\n" + p2 + "\n\n" + p3

	results, err := c.Chunk(text, ChunkOptions{ChunkSize: 80, ChunkOverlap: 20})
	require.NoError(t, err)
	assert.Greater(t, len(results), 1)

	if len(results) > 1 {
		assert.Contains(t, results[1].Content, "b")
	}
}

func TestRecursiveChunkerEmpty(t *testing.T) {
	c := NewRecursiveChunker()
	results, err := c.Chunk("", ChunkOptions{ChunkSize: 100})
	require.NoError(t, err)
	assert.Nil(t, results)
}

func TestRecursiveChunkerDefaults(t *testing.T) {
	c := NewRecursiveChunker()
	results, err := c.Chunk("hello world", ChunkOptions{})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, 0, results[0].Index)
}

func TestMarkdownAwareChunkerByHeadings(t *testing.T) {
	c := NewMarkdownAwareChunker()
	text := `# Title

Content under title.

## Section A

Content under section A.

## Section B

Content under section B.`

	results, err := c.Chunk(text, ChunkOptions{ChunkSize: 500, ChunkOverlap: 0})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(results), 2)

	var contents []string
	for _, r := range results {
		contents = append(contents, r.Content)
	}
	joined := strings.Join(contents, " ")
	assert.Contains(t, joined, "Content under section A")
	assert.Contains(t, joined, "Content under section B")
}

func TestMarkdownAwareChunkerHeadingPath(t *testing.T) {
	c := NewMarkdownAwareChunker()
	text := `# Title

Content.

## Sub

More content.`

	results, err := c.Chunk(text, ChunkOptions{ChunkSize: 500, ChunkOverlap: 0})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(results), 1)

	foundSub := false
	for _, r := range results {
		if r.Metadata != nil {
			if path, ok := r.Metadata["heading_path"].(string); ok && strings.Contains(path, "Sub") {
				foundSub = true
			}
		}
	}
	assert.True(t, foundSub)
}

func TestMarkdownAwareChunkerLargeSection(t *testing.T) {
	c := NewMarkdownAwareChunker()
	words := make([]string, 200)
	for i := range words {
		words[i] = "word"
	}
	largeContent := strings.Join(words, " ")
	text := "# Section\n\n" + largeContent

	results, err := c.Chunk(text, ChunkOptions{ChunkSize: 100, ChunkOverlap: 0})
	require.NoError(t, err)
	assert.Greater(t, len(results), 1)
}

func TestMarkdownAwareChunkerEmpty(t *testing.T) {
	c := NewMarkdownAwareChunker()
	results, err := c.Chunk("", ChunkOptions{ChunkSize: 100})
	require.NoError(t, err)
	assert.Nil(t, results)
}

func TestMarkdownAwareChunkerNoHeadings(t *testing.T) {
	c := NewMarkdownAwareChunker()
	text := "Just plain text without any headings."
	results, err := c.Chunk(text, ChunkOptions{ChunkSize: 500, ChunkOverlap: 0})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Contains(t, results[0].Content, "Just plain text")
}

func TestChunkResultIndex(t *testing.T) {
	c := NewRecursiveChunker()
	p1 := strings.Repeat("a", 50)
	p2 := strings.Repeat("b", 50)
	p3 := strings.Repeat("c", 50)
	text := p1 + "\n\n" + p2 + "\n\n" + p3

	results, err := c.Chunk(text, ChunkOptions{ChunkSize: 80, ChunkOverlap: 0})
	require.NoError(t, err)
	for i, r := range results {
		assert.Equal(t, i, r.Index)
	}
}
