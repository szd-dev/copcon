package mcp

import (
	"context"
	"log/slog"

	"github.com/copcon/core/capabilities"
	"github.com/copcon/core/hook"
	"github.com/copcon/core/tool"
)

type MCPModule struct {
	configs []MCPServerConfig
	mgr     *ConnectionManager
}

var _ capabilities.ModuleCapability = (*MCPModule)(nil)

func NewMCPModule(configs []MCPServerConfig) *MCPModule {
	return &MCPModule{configs: configs, mgr: NewConnectionManager()}
}

func NewMCPModuleWithManager(configs []MCPServerConfig, mgr *ConnectionManager) *MCPModule {
	return &MCPModule{configs: configs, mgr: mgr}
}

func (m *MCPModule) Name() string                      { return "modules.mcp" }
func (m *MCPModule) Type() capabilities.CapabilityType  { return capabilities.CapabilityTypeModule }
func (m *MCPModule) DependsOn() []string                { return nil }

func (m *MCPModule) NewHooks(deps capabilities.CapabilityDeps) ([]hook.Hook, error) {
	return nil, nil
}

func (m *MCPModule) NewTools(deps capabilities.CapabilityDeps) ([]tool.Tool, error) {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}

	var allTools []tool.Tool

	for _, cfg := range m.configs {
		session, err := m.mgr.GetSession(cfg.Name)
		if err != nil {
			ctx := context.Background()
			session, err = m.mgr.Connect(ctx, cfg)
			if err != nil {
				logger.Warn("mcp_connect_failed", "server", cfg.Name, "error", err)
				continue
			}
		}

		ctx := context.Background()
		listResult, err := session.ListTools(ctx, nil)
		if err != nil {
			logger.Warn("mcp_list_tools_failed", "server", cfg.Name, "error", err)
			continue
		}

		callFunc := buildMCPToolCallFunc(session)

		for _, mcpTool := range listResult.Tools {
			if !isToolAllowed(mcpTool.Name, cfg.AllowedTools) {
				continue
			}

			info := MCPToolInfo{
				Name:        mcpTool.Name,
				Description: mcpTool.Description,
				InputSchema: convertSchema(mcpTool.InputSchema),
				ServerName:  cfg.Name,
			}
			allTools = append(allTools, NewMCPToolWrapper(info, callFunc))
		}
	}

	return allTools, nil
}

func isToolAllowed(name string, config *AllowedToolsConfig) bool {
	if config == nil {
		return true
	}
	if len(config.Include) > 0 {
		found := false
		for _, inc := range config.Include {
			if inc == name {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	for _, exc := range config.Exclude {
		if exc == name {
			return false
		}
	}
	return true
}

func convertSchema(schema any) map[string]any {
	switch v := schema.(type) {
	case map[string]any:
		return v
	default:
		return map[string]any{"type": "object"}
	}
}

func (m *MCPModule) ConnectionManager() *ConnectionManager {
	return m.mgr
}
