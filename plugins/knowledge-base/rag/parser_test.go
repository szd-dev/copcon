package kbrag

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultParserMarkdown(t *testing.T) {
	p := NewDefaultParser()
	content, err := os.ReadFile("testdata/sample.md")
	require.NoError(t, err)

	text, err := p.Parse(context.Background(), content, "text/markdown")
	require.NoError(t, err)
	assert.Contains(t, text, "# Test Document")
	assert.Contains(t, text, "## Section One")
}

func TestDefaultParserText(t *testing.T) {
	p := NewDefaultParser()
	content, err := os.ReadFile("testdata/sample.txt")
	require.NoError(t, err)

	text, err := p.Parse(context.Background(), content, "text/plain")
	require.NoError(t, err)
	assert.Contains(t, text, "Hello, World!")
	assert.Contains(t, text, "日本語テスト")
}

func TestDefaultParserHTML(t *testing.T) {
	p := NewDefaultParser()
	content, err := os.ReadFile("testdata/sample.html")
	require.NoError(t, err)

	text, err := p.Parse(context.Background(), content, "text/html")
	require.NoError(t, err)
	assert.Contains(t, text, "Test Page")
	assert.Contains(t, text, "test paragraph")
	assert.NotContains(t, text, "var x")
}

func TestDefaultParserUnsupported(t *testing.T) {
	p := NewDefaultParser()
	_, err := p.Parse(context.Background(), []byte{}, "application/octet-stream")
	assert.Error(t, err)
}

func TestDefaultParserFallbackText(t *testing.T) {
	p := NewDefaultParser()
	text, err := p.Parse(context.Background(), []byte("hello"), "text/csv")
	require.NoError(t, err)
	assert.Equal(t, "hello", text)
}

func TestDefaultParserRegisterCustom(t *testing.T) {
	p := NewDefaultParser()
	custom := &markdownParser{}
	p.RegisterParser("application/custom", custom)
	text, err := p.Parse(context.Background(), []byte("custom content"), "application/custom")
	require.NoError(t, err)
	assert.Equal(t, "custom content", text)
}

func TestMarkdownParserPassthrough(t *testing.T) {
	p := &markdownParser{}
	text, err := p.Parse(context.Background(), []byte("# Hello\n\nWorld"), "text/markdown")
	require.NoError(t, err)
	assert.Equal(t, "# Hello\n\nWorld", text)
}

func TestTextParserCRLF(t *testing.T) {
	p := &textParser{}
	text, err := p.Parse(context.Background(), []byte("line1\r\nline2\r\n"), "text/plain")
	require.NoError(t, err)
	assert.Equal(t, "line1\nline2\n", text)
}

func TestTextParserBOM(t *testing.T) {
	p := &textParser{}
	content := append([]byte{0xEF, 0xBB, 0xBF}, []byte("hello")...)
	text, err := p.Parse(context.Background(), content, "text/plain")
	require.NoError(t, err)
	assert.Equal(t, "hello", text)
}

func TestHTMLParserStripsScript(t *testing.T) {
	p := &htmlParser{}
	html := `<html><body><p>visible</p><script>hidden</script></body></html>`
	text, err := p.Parse(context.Background(), []byte(html), "text/html")
	require.NoError(t, err)
	assert.Contains(t, text, "visible")
	assert.NotContains(t, text, "hidden")
}

func TestHTMLParserStripsStyle(t *testing.T) {
	p := &htmlParser{}
	html := `<html><body><p>content</p><style>.x{color:red}</style></body></html>`
	text, err := p.Parse(context.Background(), []byte(html), "text/html")
	require.NoError(t, err)
	assert.Contains(t, text, "content")
	assert.NotContains(t, text, "color")
}

func TestHTMLParserPreservesStructure(t *testing.T) {
	p := &htmlParser{}
	html := `<h1>Title</h1><p>Para 1</p><p>Para 2</p>`
	text, err := p.Parse(context.Background(), []byte(html), "text/html")
	require.NoError(t, err)
	assert.Contains(t, text, "Title")
	assert.Contains(t, text, "Para 1")
	assert.Contains(t, text, "Para 2")
}
