package kubernetes

import (
	"fmt"
	"net/url"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	errors "github.com/rotisserie/eris"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/solo-io/gloo/projects/gloo/pkg/discovery"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	corecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"k8s.io/client-go/kubernetes"
)

var _ discovery.DiscoveryPlugin = new(plugin)
var _ plugins.UpstreamPlugin = new(plugin)

type plugin struct {
	kube kubernetes.Interface

	UpstreamConverter UpstreamConverter

	kubeCoreCache corecache.KubeCoreCache

	settings *v1.Settings
}

func (p *plugin) Resolve(u *v1.Upstream) (*url.URL, error) {
	kubeSpec, ok := u.GetUpstreamType().(*v1.Upstream_Kube)
	if !ok {
		return nil, nil
	}

	return url.Parse(fmt.Sprintf("tcp://%v.%v.svc.cluster.local:%v", kubeSpec.Kube.GetServiceName(), kubeSpec.Kube.GetServiceNamespace(), kubeSpec.Kube.GetServicePort()))
}

func NewPlugin(kube kubernetes.Interface, kubeCoreCache corecache.KubeCoreCache) plugins.Plugin {
	return &plugin{
		kube:              kube,
		UpstreamConverter: DefaultUpstreamConverter(),
		kubeCoreCache:     kubeCoreCache,
	}
}

func (p *plugin) Init(params plugins.InitParams) error {
	p.settings = params.Settings
	return nil
}

func (p *plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoy_config_cluster_v3.Cluster) error {
	// not ours
	kube, ok := in.GetUpstreamType().(*v1.Upstream_Kube)
	if !ok {
		return nil
	}

	// configure the cluster to use EDS:ADS and call it a day
	xds.SetEdsOnCluster(out, p.settings)
	upstreamRef := in.GetMetadata().Ref()

	// Lister functions obfuscate the typical (val, ok) pair returned values of maps, so we have to do a nil check instead.
	lister := p.kubeCoreCache.NamespacedServiceLister(kube.Kube.GetServiceNamespace())
	if lister == nil {
		return errors.Errorf("Upstream %s references the service \"%s\" which has an invalid ServiceNamespace \"%s\".",
			upstreamRef.String(), kube.Kube.GetServiceName(), kube.Kube.GetServiceNamespace())
	}

	svcs, err := lister.List(labels.NewSelector())
	if err != nil {
		return err
	}
	for _, s := range svcs {
		if s.Name == kube.Kube.GetServiceName() {
			return nil
		}
	}

	return errors.Errorf("Upstream %s references the service \"%s\" which does not exist in namespace \"%s\"",
		upstreamRef.String(), kube.Kube.GetServiceName(), kube.Kube.GetServiceNamespace())

}
