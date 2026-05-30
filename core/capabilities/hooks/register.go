package hooks

import "github.com/copcon/core/capabilities"

func RegisterAll(r *capabilities.Registry) {
	r.Register(&loggingHookCapability{})
	r.Register(&todoInjectionHookCapability{})
	r.Register(&tracingHookCapability{})
}
