package kbrag

import (
	"bytes"
	"context"
	"io"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

type textParser struct{}

func (p *textParser) Parse(ctx context.Context, content []byte, mimetype string) (string, error) {
	text := handleBOM(content)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return text, nil
}

func handleBOM(data []byte) string {
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
		return string(data)
	}

	if len(data) >= 2 {
		if data[0] == 0xFF && data[1] == 0xFE {
			decoder := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder()
			result, err := decoder.Bytes(data)
			if err == nil {
				return string(result)
			}
		}
		if data[0] == 0xFE && data[1] == 0xFF {
			decoder := unicode.UTF16(unicode.BigEndian, unicode.UseBOM).NewDecoder()
			result, err := decoder.Bytes(data)
			if err == nil {
				return string(result)
			}
		}
	}

	if !utf8.Valid(data) {
		reader := transform.NewReader(bytes.NewReader(data), unicode.UTF8.NewDecoder())
		transformed, err := io.ReadAll(reader)
		if err == nil {
			return string(transformed)
		}
	}

	return string(data)
}