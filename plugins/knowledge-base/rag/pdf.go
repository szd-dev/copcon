package kbrag

import (
	"bytes"
	"context"

	"github.com/dslipak/pdf"
)

type pdfParser struct{}

func (p *pdfParser) Parse(ctx context.Context, content []byte, mimetype string) (string, error) {
	reader, err := pdf.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return "", err
	}

	var text string
	n := reader.NumPage()
	for i := 1; i <= n; i++ {
		page := reader.Page(i)
		if page.V.IsNull() {
			continue
		}
		pageText, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		text += pageText + "\n"
	}
	return text, nil
}
