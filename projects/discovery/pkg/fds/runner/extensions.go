package runner

import "github.com/solo-io/gloo/projects/discovery/pkg/fds"

type RunExtensions struct {
	DiscoveryFactoryFuncs []func() fds.FunctionDiscoveryFactory
}
