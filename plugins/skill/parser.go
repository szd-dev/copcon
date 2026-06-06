package skill

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	skilltypes "github.com/copcon/core/capabilities/skill"
	"gopkg.in/yaml.v3"
)

var skillNamePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

var resourceCategories = []string{"scripts", "references", "assets"}

func ParseSkill(dirPath string) (*skilltypes.Skill, error) {
	info, err := os.Stat(dirPath)
	if err != nil {
		return nil, fmt.Errorf("skill directory not found: %s: %w", dirPath, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("expected directory, got file: %s", dirPath)
	}

	dirName := filepath.Base(dirPath)
	if !skillNamePattern.MatchString(dirName) {
		return nil, fmt.Errorf("invalid skill directory name %q: must match %s", dirName, skillNamePattern.String())
	}

	skillMDPath := filepath.Join(dirPath, "SKILL.md")
	content, err := os.ReadFile(skillMDPath)
	if err != nil {
		return nil, fmt.Errorf("SKILL.md not found in %s", dirPath)
	}

	name, description, license, metadata, allowedTools, body, err := parseSKILLmd(string(content))
	if err != nil {
		return nil, fmt.Errorf("parsing SKILL.md in %s: %w", dirPath, err)
	}

	if name != dirName {
		return nil, fmt.Errorf("skill name %q does not match directory name %q", name, dirName)
	}

	if strings.TrimSpace(description) == "" {
		return nil, fmt.Errorf("skill %q: description must not be empty", name)
	}

	absPath, _ := filepath.Abs(dirPath)

	resourceFiles, err := scanResourceFiles(dirPath)
	if err != nil {
		return nil, fmt.Errorf("scanning resource files in %s: %w", dirPath, err)
	}

	return &skilltypes.Skill{
		Name:          name,
		Description:   description,
		License:       license,
		Metadata:      metadata,
		AllowedTools:  allowedTools,
		Instructions:  body,
		DirPath:       absPath,
		Source:        absPath,
		ResourceFiles: resourceFiles,
	}, nil
}

func ParseSkillDir(rootPath string, logger *slog.Logger) ([]*skilltypes.Skill, error) {
	entries, err := os.ReadDir(rootPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading skill directory %s: %w", rootPath, err)
	}

	var skills []*skilltypes.Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillPath := filepath.Join(rootPath, entry.Name())
		s, err := ParseSkill(skillPath)
		if err != nil {
			if logger != nil {
				logger.Warn("skipping invalid skill directory", "path", skillPath, "error", err)
			}
			continue
		}
		skills = append(skills, s)
	}

	return skills, nil
}

func parseSKILLmd(content string) (name, description, license string, metadata map[string]string, allowedTools, body string, err error) {
	if !strings.HasPrefix(content, "---") {
		return "", "", "", nil, "", "", fmt.Errorf("SKILL.md must start with YAML frontmatter delimiter '---'")
	}

	rest := content[3:]
	endIdx := strings.Index(rest, "\n---")
	if endIdx < 0 {
		return "", "", "", nil, "", "", fmt.Errorf("SKILL.md missing closing YAML frontmatter delimiter '---'")
	}

	frontmatter := rest[:endIdx]
	body = strings.TrimSpace(rest[endIdx+4:])

	var fm map[string]any
	if err := yaml.Unmarshal([]byte(frontmatter), &fm); err != nil {
		return "", "", "", nil, "", "", fmt.Errorf("invalid YAML frontmatter: %w", err)
	}

	name = stringValue(fm["name"])
	description = stringValue(fm["description"])
	license = stringValue(fm["license"])
	allowedTools = stringValue(fm["allowed-tools"])

	metadata = make(map[string]string)
	if md, ok := fm["metadata"]; ok {
		if mdMap, ok := md.(map[string]any); ok {
			for k, v := range mdMap {
				metadata[k] = fmt.Sprintf("%v", v)
			}
		}
	}

	return name, description, license, metadata, allowedTools, body, nil
}

func stringValue(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func scanResourceFiles(dirPath string) ([]skilltypes.ResourceFile, error) {
	var files []skilltypes.ResourceFile

	for _, category := range resourceCategories {
		catDir := filepath.Join(dirPath, category)
		entries, err := os.ReadDir(catDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			relPath := filepath.Join(category, entry.Name())
			absPath := filepath.Join(dirPath, relPath)
			files = append(files, skilltypes.ResourceFile{
				Name:     relPath,
				Path:     absPath,
				Category: category,
			})
		}
	}

	return files, nil
}