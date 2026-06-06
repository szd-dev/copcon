package skill

import (
	"log/slog"
	"os"
	"path/filepath"

	skilltypes "github.com/copcon/core/capabilities/skill"
)

type Discoverer struct {
	paths  []string
	logger *slog.Logger
}

func NewDiscoverer(projectRoot string, extraPaths []string, logger *slog.Logger) *Discoverer {
	if logger == nil {
		logger = slog.Default()
	}

	var defaultPaths []string

	if projectRoot != "" {
		defaultPaths = append(defaultPaths,
			filepath.Join(projectRoot, ".copcon", "skills"),
			filepath.Join(projectRoot, ".agents", "skills"),
		)
	}

	homeDir, err := os.UserHomeDir()
	if err == nil && homeDir != "" {
		defaultPaths = append(defaultPaths,
			filepath.Join(homeDir, ".copcon", "skills"),
			filepath.Join(homeDir, ".agents", "skills"),
		)
	}

	paths := make([]string, 0, len(extraPaths)+len(defaultPaths))
	paths = append(paths, extraPaths...)
	paths = append(paths, defaultPaths...)

	return &Discoverer{
		paths:  paths,
		logger: logger,
	}
}

func (d *Discoverer) AddPath(path string) {
	d.paths = append(d.paths, path)
}

func (d *Discoverer) PrependPath(path string) {
	d.paths = append([]string{path}, d.paths...)
}

func (d *Discoverer) Discover() ([]*skilltypes.Skill, error) {
	seen := make(map[string]bool)
	var result []*skilltypes.Skill

	for _, path := range d.paths {
		skills, err := ParseSkillDir(path, d.logger)
		if err != nil {
			d.logger.Warn("skipping skill search path", "path", path, "error", err)
			continue
		}
		for _, s := range skills {
			if seen[s.Name] {
				continue
			}
			seen[s.Name] = true
			result = append(result, s)
		}
	}

	return result, nil
}
