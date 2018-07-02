package bootstrap

import (
	"github.com/solo-io/gloo/pkg/bootstrap"
)

type Options struct {
	bootstrap.Options
	UpstreamDiscoveryOptions UpstreamDiscoveryOptions
}

type UpstreamDiscoveryOptions struct {
	EnableDiscoveryForKubernetes bool
	EnableDiscoveryForCopilot    bool
	EnableDiscoveryForConsul     bool
}

func (opts UpstreamDiscoveryOptions) DiscoveryEnabled() bool {
	return opts.EnableDiscoveryForKubernetes ||
		opts.EnableDiscoveryForCopilot ||
		opts.EnableDiscoveryForConsul
}
