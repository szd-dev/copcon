package tools

import "github.com/copcon/core/capabilities"

func RegisterAll(r *capabilities.Registry) {
	r.Register(&codeExecutorCapability{})
	r.Register(&shellExecutorCapability{})
	r.Register(&fileOpsCapability{})
	r.Register(&asyncCapability{})
	r.Register(&todoCapability{})
	r.Register(&delegateCapability{})
	r.Register(&confirmActionCapability{})
	r.Register(&askUserCapability{})
}
