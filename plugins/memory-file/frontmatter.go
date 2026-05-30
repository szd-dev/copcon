package memoryfile

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Frontmatter holds the YAML metadata at the top of a memory MD file.
type Frontmatter struct {
	Name      string            `yaml:"name"`
	Category  string            `yaml:"category"`
	Importance float64          `yaml:"importance,omitempty"`
	CreatedAt time.Time         `yaml:"created_at"`
	UpdatedAt time.Time         `yaml:"updated_at"`
	Tags      []string          `yaml:"tags,omitempty"`
	Metadata  map[string]string `yaml:"metadata,omitempty"`
}

// ParseFrontmatter extracts the YAML frontmatter from a markdown file.
// The file must start with "---" delimiters. Returns the parsed frontmatter
// and the remaining body content after the closing "---".
func ParseFrontmatter(data []byte) (Frontmatter, string, error) {
	var fm Frontmatter

	if !bytes.HasPrefix(data, []byte("---\n")) && !bytes.HasPrefix(data, []byte("---\r\n")) {
		return fm, string(data), nil
	}

	end := bytes.Index(data[3:], []byte("---"))
	if end == -1 {
		return fm, "", fmt.Errorf("frontmatter closing delimiter not found")
	}

	fmBytes := data[3 : 3+end]
	body := string(bytes.TrimSpace(data[3+end+3:]))

	if err := yaml.Unmarshal(fmBytes, &fm); err != nil {
		return fm, "", fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	return fm, body, nil
}

// SerializeFrontmatter combines frontmatter and body into a complete MD file
// with YAML frontmatter delimiters.
func SerializeFrontmatter(fm Frontmatter, body string) []byte {
	var buf strings.Builder
	buf.WriteString("---\n")

	encoder := yaml.NewEncoder(&buf)
	if err := encoder.Encode(&fm); err != nil {
		buf.WriteString(fmt.Sprintf("name: %q\n", fm.Name))
		buf.WriteString(fmt.Sprintf("category: %q\n", fm.Category))
		buf.WriteString(fmt.Sprintf("created_at: %q\n", fm.CreatedAt.Format(time.RFC3339)))
		buf.WriteString(fmt.Sprintf("updated_at: %q\n", fm.UpdatedAt.Format(time.RFC3339)))
	} else {
		encoder.Close()
	}

	buf.WriteString("---\n")
	buf.WriteString(body)

	return []byte(buf.String())
}
