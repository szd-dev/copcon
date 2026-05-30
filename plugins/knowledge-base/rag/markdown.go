package kbrag

import "context"

type markdownParser struct{}

func (p *markdownParser) Parse(ctx context.Context, content []byte, mimetype string) (string, error) {
	return string(content), nil
}
