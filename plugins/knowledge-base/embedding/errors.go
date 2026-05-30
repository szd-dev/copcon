package kbembedding

import "errors"

var (
	// ErrUnsupportedBackend is returned when the configured backend type is not
	// recognised or not yet implemented.
	ErrUnsupportedBackend = errors.New("unsupported embedding backend")

	// ErrEmptyText is returned when an embed request receives an empty input.
	ErrEmptyText = errors.New("empty text provided for embedding")

	// ErrDimensionMismatch is returned when the returned vector dimension does
	// not match the expected or configured dimension.
	ErrDimensionMismatch = errors.New("embedding dimension mismatch")
)