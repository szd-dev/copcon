package rag

import (
	"context"
	"fmt"
	"strings"
)

type DefaultParser struct {
	parsers map[string]Parser
}

func NewDefaultParser() *DefaultParser {
	dp := &DefaultParser{
		parsers: make(map[string]Parser),
	}
	dp.parsers["application/pdf"] = &pdfParser{}
	dp.parsers["text/markdown"] = &markdownParser{}
	dp.parsers["text/x-markdown"] = &markdownParser{}
	for _, mt := range textMimetypes {
		dp.parsers[mt] = &textParser{}
	}
	dp.parsers["text/html"] = &htmlParser{}
	dp.parsers["application/xhtml+xml"] = &htmlParser{}
	return dp
}

var textMimetypes = []string{
	"text/plain",
	"text/csv",
	"text/log",
	"application/json",
	"application/xml",
	"text/xml",
	"text/yaml",
	"application/yaml",
}

func (p *DefaultParser) Parse(ctx context.Context, content []byte, mimetype string) (string, error) {
	if parser, ok := p.parsers[mimetype]; ok {
		return parser.Parse(ctx, content, mimetype)
	}
	if strings.HasPrefix(mimetype, "text/") {
		return (&textParser{}).Parse(ctx, content, mimetype)
	}
	return "", fmt.Errorf("unsupported mimetype: %s", mimetype)
}

func (p *DefaultParser) RegisterParser(mimetype string, parser Parser) {
	p.parsers[mimetype] = parser
}
