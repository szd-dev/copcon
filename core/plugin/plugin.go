// Package plugin provides the core plugin interface and global registration
// pools for the strangler-fig migration from the legacy capabilities system.
//
// Plugins follow a two-phase lifecycle:
//  1. Register — plugin is added to the registry; no dependencies available.
//  2. Build   — PluginDeps are injected and Init is called.
//
// This package coexists with core/capabilities until Task 10.
package plugin

import (
	"log/slog"

	"github.com/copcon/core/agent"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/storage"
	"github.com/copcon/core/tool"
)

// Plugin is the top-level interface for first-class plugins.
// Name/Tools/Hooks are called during Register; Init is called during Build.
type Plugin interface {
	Name() string
	Tools() []tool.Tool
	Hooks() []hook.Hook
	Init(deps PluginDeps) error
}

// PluginDeps holds lazily-injected dependencies for the Build phase.
// Mirrors CapabilityDeps from core/capabilities/registry.go.
type PluginDeps struct {
	SessionStore        storage.SessionStore
	MessageStore        storage.MessageStore
	TodoStore           storage.TodoStore
	AgentRegistry       agent.AgentRegistry
	Engine              interface{} // AgentEngine — typed as interface{} to avoid circular imports
	Logger              *slog.Logger
	AgentKnowledgeBases map[string][]string // agentID → KB IDs
}
