package mcp

import (
	"testing"

	"github.com/copcon/core/capabilities"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterCapabilities(t *testing.T) {
	reg := capabilities.NewRegistry()

	configs := []MCPServerConfig{
		{Name: "test-server", Type: TransportStdio, Command: "echo"},
	}

	RegisterCapabilities(reg, configs)

	got, ok := reg.Get(CapabilityName)
	require.True(t, ok)
	require.NotNil(t, got)

	module, ok := got.(*MCPModule)
	require.True(t, ok)
	assert.Equal(t, CapabilityName, module.Name())
}

func TestRegisterCapabilitiesEmpty(t *testing.T) {
	reg := capabilities.NewRegistry()

	RegisterCapabilities(reg, nil)

	got, ok := reg.Get(CapabilityName)
	require.True(t, ok)
	require.NotNil(t, got)

	module, ok := got.(*MCPModule)
	require.True(t, ok)
	assert.Equal(t, CapabilityName, module.Name())
}

func TestCapabilityName(t *testing.T) {
	assert.Equal(t, "modules.mcp", CapabilityName)
}
