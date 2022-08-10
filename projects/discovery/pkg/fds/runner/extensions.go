package runner

import "github.com/solo-io/gloo/projects/discovery/pkg/fds"

// RunExtensions represent the properties that can be injected into an FDS Runner
// These properties are the injection point for Enterprise functionality
type RunExtensions struct {
	// DiscoveryFactoryFuncs are a set of functions which return fds.FunctionDiscoveryFactory's
	DiscoveryFactoryFuncs []func() fds.FunctionDiscoveryFactory
}
