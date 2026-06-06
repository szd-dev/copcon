package skill

import (
	"log/slog"

	"github.com/copcon/core/capabilities"
)

// RegisterCapabilities registers the Skill module into the given Registry.
func RegisterCapabilities(r *capabilities.Registry, cfg Config) {
	if err := r.Register(NewSkillModule(cfg)); err != nil {
		slog.Warn("skill module registration", "error", err)
	}
}