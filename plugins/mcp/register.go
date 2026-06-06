package mcp

import (
	"log/slog"

	"github.com/copcon/core/capabilities"
)

// CapabilityName is the identifier for the MCP module capability.
const CapabilityName = "modules.mcp"

// RegisterCapabilities creates and registers the MCP module with the given server configs.
func RegisterCapabilities(r *capabilities.Registry, configs []MCPServerConfig) {
	if err := r.Register(NewMCPModule(configs)); err != nil {
		slog.Warn("mcp module registration", "error", err)
	}
}
