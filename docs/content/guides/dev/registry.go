//go:build ignore
// +build ignore

package docs_demo

// package registry

import (
	"github.com/solo-io/gloo/projects/controller/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/controller/pkg/plugins"
	"github.com/solo-io/gloo/projects/controller/pkg/plugins/aws"
	"github.com/solo-io/gloo/projects/controller/pkg/plugins/azure"
	"github.com/solo-io/gloo/projects/controller/pkg/plugins/basicroute"
	"github.com/solo-io/gloo/projects/controller/pkg/plugins/consul"
	"github.com/solo-io/gloo/projects/controller/pkg/plugins/cors"
	"github.com/solo-io/gloo/projects/controller/pkg/plugins/faultinjection"

	// add our plugin's import here:
	"github.com/solo-io/gloo/projects/controller/pkg/plugins/gce"
	"github.com/solo-io/gloo/projects/controller/pkg/plugins/grpc"
	"github.com/solo-io/gloo/projects/controller/pkg/plugins/hcm"
	"github.com/solo-io/gloo/projects/controller/pkg/plugins/kubernetes"
	"github.com/solo-io/gloo/projects/controller/pkg/plugins/linkerd"
	"github.com/solo-io/gloo/projects/controller/pkg/plugins/loadbalancer"
	"github.com/solo-io/gloo/projects/controller/pkg/plugins/rest"
	"github.com/solo-io/gloo/projects/controller/pkg/plugins/static"
	"github.com/solo-io/gloo/projects/controller/pkg/plugins/stats"
	"github.com/solo-io/gloo/projects/controller/pkg/plugins/transformation"
	"github.com/solo-io/gloo/projects/controller/pkg/plugins/upstreamconn"
	"github.com/solo-io/gloo/projects/controller/pkg/plugins/upstreamssl"
)

type registry struct {
	plugins []plugins.Plugin
}

var globalRegistry = func(opts bootstrap.Opts) *registry {
	transformationPlugin := transformation.NewPlugin()
	reg := &registry{}
	// plugins should be added here
	reg.plugins = append(reg.plugins,
		loadbalancer.NewPlugin(),
		upstreamconn.NewPlugin(),
		upstreamssl.NewPlugin(),
		azure.NewPlugin(&transformationPlugin.RequireTransformationFilter),
		aws.NewPlugin(&transformationPlugin.RequireTransformationFilter, &transformationPlugin.RequireEarlyTransformation),
		rest.NewPlugin(&transformationPlugin.RequireTransformationFilter),
		hcm.NewPlugin(),
		static.NewPlugin(),
		transformationPlugin,
		consul.NewPlugin(),
		grpc.NewPlugin(&transformationPlugin.RequireTransformationFilter),
		faultinjection.NewPlugin(),
		basicroute.NewPlugin(),
		cors.NewPlugin(),
		linkerd.NewPlugin(),
		stats.NewPlugin(),
		// and our plugin goes here
		gce.NewPlugin(),
	)
	if opts.KubeClient != nil {
		reg.plugins = append(reg.plugins, kubernetes.NewPlugin(opts.KubeClient))
	}
	for _, pluginExtension := range pluginExtensions {
		reg.plugins = append(reg.plugins, pluginExtension())
	}

	return reg
}

func Plugins(opts bootstrap.Opts, pluginExtensions ...plugins.Plugin) []plugins.Plugin {
	return globalRegistry(opts, pluginExtensions...).plugins
}
