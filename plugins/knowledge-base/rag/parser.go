package kbrag

import "context"

type Parser interface {
	Parse(ctx context.Context, content []byte, mimetype string) (string, error)
}
