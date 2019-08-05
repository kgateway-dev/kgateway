package setup

import (
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
)

type Opts struct {
	ClusterIngressProxyAddress string
	KnativeProxyAddress        string
	WriteNamespace             string
	WatchNamespaces            []string
	Proxies                    factory.ResourceClientFactory
	Upstreams                  factory.ResourceClientFactory
	Secrets                    factory.ResourceClientFactory
	WatchOpts                  clients.WatchOpts
	EnableKnative              bool
	KnativeVersion             string
	DisableKubeIngress         bool
}
