package consul

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/solo-io/gloo/projects/gloo/pkg/discovery"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/hashicorp/consul/api"
	"github.com/rotisserie/eris"

	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
)

var (
	_ discovery.DiscoveryPlugin = new(plugin)
	_ plugins.UpstreamPlugin    = new(plugin)
	_ plugins.RouteActionPlugin = new(plugin)
)

const (
	ExtensionName             = "consul"
	DefaultDnsAddress         = "127.0.0.1:8600"
	DefaultDnsPollingInterval = 5 * time.Second
	DefaultTlsTagName         = "glooUseTls"
)

var (
	UnformattedErrorMsg = "Consul settings specify automatic detection of TLS services, " +
		"but the rootCA resource's name/namespace are not properly specified: {%s}"
	ConsulTlsInputError = func(nsString string) error {
		return eris.Errorf(UnformattedErrorMsg, nsString)
	}
)

type plugin struct {
	client                          consul.ConsulWatcher
	resolver                        DnsResolver
	dnsPollingInterval              time.Duration
	consulUpstreamDiscoverySettings *v1.Settings_ConsulUpstreamDiscoveryConfiguration
	settings                        *v1.Settings
}

func NewPlugin(client consul.ConsulWatcher, resolver DnsResolver, dnsPollingInterval *time.Duration) *plugin {
	pollingInterval := DefaultDnsPollingInterval
	if dnsPollingInterval != nil {
		pollingInterval = *dnsPollingInterval
	}
	return &plugin{client: client, resolver: resolver, dnsPollingInterval: pollingInterval}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Resolve(u *v1.Upstream) (*url.URL, error) {
	consulSpec, ok := u.GetUpstreamType().(*v1.Upstream_Consul)
	if !ok {
		return nil, nil
	}

	spec := consulSpec.Consul

	// default to first datacenter
	var dc string
	if len(spec.GetDataCenters()) > 0 {
		dc = spec.GetDataCenters()[0]
	}

	instances, _, err := p.client.Service(spec.GetServiceName(), "", &api.QueryOptions{Datacenter: dc, RequireConsistent: true})
	if err != nil {
		return nil, eris.Wrapf(err, "getting service from catalog")
	}

	scheme := "http"
	if u.GetSslConfig() != nil {
		scheme = "https"
	}

	// Match service instances (consul endpoints) to gloo upstreams. A match is found if the upstream's
	// InstanceTags array is a subset of the serviceInstance's tags, or always if InstanceTags is empty.
	// If the upstream's instanceBlackListTags array is non-empty, then there must also be no matches between
	// this and the service instances tags.
	//
	// There's no coordination between upstreams when matching. This makes it a little awkward to sort
	// consul serviceInstance's among upstreams if we have any upstream with an empty InstanceTags array,
	// since that will also auto-match with serviceInstances that had matching tags for another upstream.
	//
	// The resulting implication is:
	// If there are multiple upstreams associated with the same consul service, each upstream MUST have a non-empty
	// InstanceTags array, and that service's serviceInstances MUST have enough tags to match them to at least one
	// service. If a serviceInstance has the tags to match into multiple upstreams, then it'll be associated with
	// multiple upstreams. This isn't always bad par se, but is not ideal when only some upstreams are secure.
	for _, inst := range instances {
		instanceMatch := len(spec.GetInstanceTags()) == 0 || matchTags(spec.GetInstanceTags(), inst.ServiceTags)
		antiInstanceMatch := len(spec.GetInstanceBlacklistTags()) == 0 || mutuallyExclusiveTags(spec.GetInstanceBlacklistTags(), inst.ServiceTags)

		if instanceMatch && antiInstanceMatch {
			ipAddresses, err := getIpAddresses(context.TODO(), inst.ServiceAddress, p.resolver)
			if err != nil {
				return nil, err
			}
			if len(ipAddresses) == 0 {
				return nil, eris.Errorf("DNS result for %s returned an empty list of IPs", inst.ServiceAddress)
			}
			// arbitrarily default to the first result
			ipAddr := ipAddresses[0]
			return url.Parse(fmt.Sprintf("%v://%v:%v", scheme, ipAddr, inst.ServicePort))
		}
	}

	return nil, eris.Errorf("service with name %s and tags %v not found", spec.GetServiceName(), spec.GetInstanceTags())
}

func (p *plugin) Init(params plugins.InitParams) error {
	p.settings = params.Settings
	p.consulUpstreamDiscoverySettings = params.Settings.GetConsulDiscovery()
	if p.consulUpstreamDiscoverySettings == nil {
		p.consulUpstreamDiscoverySettings = &v1.Settings_ConsulUpstreamDiscoveryConfiguration{UseTlsTagging: false}
	}
	// if automatic TLS discovery is enabled for consul services, make sure we have a specified tag
	// and a resource location for the validation context's root CA.
	// The tag has a default value, but the resource name/namespace must be set manually.
	if p.consulUpstreamDiscoverySettings.GetUseTlsTagging() {
		rootCa := p.consulUpstreamDiscoverySettings.GetRootCa()
		if rootCa.GetNamespace() == "" || rootCa.GetName() == "" {
			return ConsulTlsInputError(rootCa.String())
		}

		tlsTagName := p.consulUpstreamDiscoverySettings.GetTlsTagName()
		if tlsTagName == "" {
			p.consulUpstreamDiscoverySettings.TlsTagName = DefaultTlsTagName
		}
	}
	return nil
}

func (p *plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoy_config_cluster_v3.Cluster) error {
	_, ok := in.GetUpstreamType().(*v1.Upstream_Consul)
	if !ok {
		return nil
	}

	// consul upstreams use EDS
	xds.SetEdsOnCluster(out, p.settings)

	return nil
}

// make sure t1 is a subset of t2
func matchTags(t1, t2 []string) bool {
	if len(t1) > len(t2) {
		return false
	}
	for _, tag1 := range t1 {
		var found bool
		for _, tag2 := range t2 {
			if tag1 == tag2 {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// make sure t1 and t2 are mutually exclusive
func mutuallyExclusiveTags(t1, t2 []string) bool {
	for _, tag1 := range t1 {
		var found bool
		for _, tag2 := range t2 {
			if tag1 == tag2 {
				found = true
				break
			}
		}
		if found {
			return false
		}
	}
	return true
}
