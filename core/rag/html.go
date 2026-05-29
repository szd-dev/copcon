package rag

import (
	"context"
	"strings"

	"golang.org/x/net/html"
)

type htmlParser struct{}

func (p *htmlParser) Parse(ctx context.Context, content []byte, mimetype string) (string, error) {
	doc, err := html.Parse(strings.NewReader(string(content)))
	if err != nil {
		return "", err
	}
	return extractText(doc), nil
}

func extractText(n *html.Node) string {
	if n.Type == html.TextNode {
		return strings.TrimSpace(n.Data)
	}

	if n.Type == html.ElementNode {
		switch n.Data {
		case "script", "style", "noscript":
			return ""
		case "br":
			return "\n"
		case "p", "div", "h1", "h2", "h3", "h4", "h5", "h6", "li", "tr":
			var parts []string
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				t := extractText(c)
				if t != "" {
					parts = append(parts, t)
				}
			}
			text := strings.Join(parts, " ")
			if text != "" {
				return text + "\n"
			}
			return ""
		}
	}

	var parts []string
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		t := extractText(c)
		if t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, " ")
}
