package rag

type ChunkOptions struct {
	ChunkSize    int
	ChunkOverlap int
}

type ChunkResult struct {
	Content  string
	Index    int
	Metadata map[string]any
}

type Chunker interface {
	Chunk(text string, opts ChunkOptions) ([]ChunkResult, error)
}
