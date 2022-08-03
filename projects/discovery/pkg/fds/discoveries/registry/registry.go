package registry

import (
	"github.com/solo-io/gloo/projects/discovery/pkg/fds"
	"github.com/solo-io/gloo/projects/discovery/pkg/fds/discoveries/aws"
	"github.com/solo-io/gloo/projects/discovery/pkg/fds/discoveries/grpc"
	"github.com/solo-io/gloo/projects/discovery/pkg/fds/discoveries/swagger"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

type FunctionDiscoveryPlugin func(u *v1.Upstream) fds.UpstreamFunctionDiscovery

type registry struct {
	plugins []fds.FunctionDiscoveryFactory
}

var globalRegistry = func(pluginExtensions ...func() plugins.Plugin) *registry {
	reg := &registry{}
	// plugins should be added here
	reg.plugins = append(reg.plugins,
		aws.NewFunctionDiscoveryFactory(),
		grpc.NewFunctionDiscoveryFactory(),
		swagger.NewFunctionDiscoveryFactory(),
	)

	return reg
}

func Plugins() []fds.FunctionDiscoveryFactory {
	return globalRegistry().plugins
}
