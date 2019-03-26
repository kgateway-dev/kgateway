package registry

import (
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/aws"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/azure"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/basicroute"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/consul"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/cors"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/faultinjection"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/grpc"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/kubernetes"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/rest"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/upstreamssl"
)

type registry struct {
	plugins []plugins.Plugin
}

var globalRegistry = func(opts bootstrap.Opts, pluginExtensions ...plugins.Plugin) *registry {
	transformationPlugin := transformation.NewPlugin()
	reg := &registry{}
	// plugins should be added here
	reg.plugins = append(reg.plugins,
		upstreamssl.NewPlugin(),
		azure.NewPlugin(&transformationPlugin.RequireTransformationFilter),
		aws.NewPlugin(&transformationPlugin.RequireTransformationFilter),
		rest.NewPlugin(&transformationPlugin.RequireTransformationFilter),
		hcm.NewPlugin(),
		static.NewPlugin(),
		transformationPlugin,
		consul.NewPlugin(),
		grpc.NewPlugin(&transformationPlugin.RequireTransformationFilter),
		faultinjection.NewPlugin(),
		basicroute.NewPlugin(),
		cors.NewPlugin(),
	)
	if opts.KubeClient != nil {
		reg.plugins = append(reg.plugins, kubernetes.NewPlugin(opts.KubeClient))
	}
	for _, pluginExtension := range pluginExtensions {
		reg.plugins = append(reg.plugins, pluginExtension)
	}

	return reg
}

func Plugins(opts bootstrap.Opts, pluginExtensions ...plugins.Plugin) []plugins.Plugin {
	return globalRegistry(opts, pluginExtensions...).plugins
}
