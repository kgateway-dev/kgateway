package translator

import (
	"sort"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	envoyutil "github.com/envoyproxy/go-control-plane/pkg/util"

	"github.com/pkg/errors"

	"github.com/solo-io/gloo/internal/control-plane/reporter"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/endpointdiscovery"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/plugins"
	"github.com/solo-io/gloo/pkg/secretwatcher"
)

// filter virtual services for the listener
func virtualServicesForListener(listener *v1.Listener, virtualServices []*v1.VirtualService) ([]*v1.VirtualService, error) {
	var listenerVirtualServices []*v1.VirtualService
	for _, name := range listener.VirtualServices {
		var vsFound bool
		for _, vs := range virtualServices {
			if vs.Name == name {
				listenerVirtualServices = append(listenerVirtualServices, vs)
				vsFound = true
				break
			}
		}
		if !vsFound {
			return nil, errors.Errorf("virtual service %v not found for listener %v", name, listener.Name)
		}
	}
	return listenerVirtualServices, nil
}

// gets the subset of upstreams which are destinations for at least one route in at least one
// virtual service
func destinationUpstreams(allUpstreams []*v1.Upstream, virtualServices []*v1.VirtualService) []*v1.Upstream {
	destinationUpstreamNames := make(map[string]bool)
	for _, vs := range virtualServices {
		for _, route := range vs.Routes {
			dests := getAllDestinations(route)
			for _, dest := range dests {
				var upstreamName string
				switch typedDest := dest.DestinationType.(type) {
				case *v1.Destination_Upstream:
					upstreamName = typedDest.Upstream.Name
				case *v1.Destination_Function:
					upstreamName = typedDest.Function.UpstreamName
				default:
					panic("unknown destination type")
				}
				destinationUpstreamNames[upstreamName] = true
			}
		}
	}
	var destinationUpstreams []*v1.Upstream
	for _, us := range allUpstreams {
		if _, ok := destinationUpstreamNames[us.Name]; ok {
			destinationUpstreams = append(destinationUpstreams, us)
		}
	}
	return destinationUpstreams
}

func getAllDestinations(route *v1.Route) []*v1.Destination {
	var dests []*v1.Destination
	if route.SingleDestination != nil {
		dests = append(dests, route.SingleDestination)
	}
	for _, dest := range route.MultipleDestinations {
		dests = append(dests, dest.Destination)
	}
	return dests
}

func destinationEndpoints(upstreams []*v1.Upstream, allEndpoints endpointdiscovery.EndpointGroups) endpointdiscovery.EndpointGroups {
	destinationEndpoints := make(endpointdiscovery.EndpointGroups)
	for _, us := range upstreams {
		eps, ok := allEndpoints[us.Name]
		if !ok {
			continue
		}
		destinationEndpoints[us.Name] = eps
	}
	return destinationEndpoints
}

type stagedFilter struct {
	filter *envoyhttp.HttpFilter
	stage  plugins.Stage
}

func (t *Translator) constructHttpListener(name string, port uint32, filters []envoylistener.Filter) *envoyapi.Listener {
	return &envoyapi.Listener{
		Name: name,
		Address: envoycore.Address{
			Address: &envoycore.Address_SocketAddress{
				SocketAddress: &envoycore.SocketAddress{
					Protocol: envoycore.TCP,
					Address:  t.config.BindAddress,
					PortSpecifier: &envoycore.SocketAddress_PortValue{
						PortValue: port,
					},
					Ipv4Compat: true,
				},
			},
		},
		FilterChains: []envoylistener.FilterChain{{
			Filters: filters,
		}},
	}
}

const (
	sslCertificateChainKey           = "tls.crt"
	deprecatedSslCertificateChainKey = "ca_chain"
	sslPrivateKeyKey                 = "tls.key"
	deprecatedSslPrivateKeyKey       = "private_key"
)

func (t *Translator) constructHttpsListener(name string,
	port uint32,
	filters []envoylistener.Filter,
	virtualServices []*v1.VirtualService,
	virtualServiceReports []reporter.ConfigObjectReport,
	secrets secretwatcher.SecretMap) *envoyapi.Listener {

	// create the base filter chain
	// we will copy the filter chain for each virtualservice that specifies an ssl config
	var filterChains []envoylistener.FilterChain
	for _, vService := range virtualServices {
		if vService.SslConfig == nil || vService.SslConfig.SecretRef == "" {
			continue
		}
		ref := vService.SslConfig.SecretRef
		certChain, privateKey, err := getSslSecrets(ref, secrets)
		if err != nil {
			log.Warnf("skipping ssl vService with invalid secrets: %v", vService.Name)
			continue
		}
		filterChain := newSslFilterChain(certChain, privateKey, filters)
		filterChains = append(filterChains, filterChain)
	}

	return &envoyapi.Listener{
		Name: name,
		Address: envoycore.Address{
			Address: &envoycore.Address_SocketAddress{
				SocketAddress: &envoycore.SocketAddress{
					Protocol: envoycore.TCP,
					Address:  t.config.BindAddress,
					PortSpecifier: &envoycore.SocketAddress_PortValue{
						PortValue: port,
					},
					Ipv4Compat: true,
				},
			},
		},
		FilterChains: filterChains,
	}
}

func newSslFilterChain(certChain, privateKey string, filters []envoylistener.Filter) envoylistener.FilterChain {
	return envoylistener.FilterChain{
		Filters: filters,
		TlsContext: &envoyauth.DownstreamTlsContext{
			CommonTlsContext: &envoyauth.CommonTlsContext{
				// default params
				TlsParams: &envoyauth.TlsParameters{},
				// TODO: configure client certificates
				TlsCertificates: []*envoyauth.TlsCertificate{
					{
						CertificateChain: &envoycore.DataSource{
							Specifier: &envoycore.DataSource_InlineString{
								InlineString: certChain,
							},
						},
						PrivateKey: &envoycore.DataSource{
							Specifier: &envoycore.DataSource_InlineString{
								InlineString: privateKey,
							},
						},
					},
				},
			},
		},
	}
}

func (t *Translator) createHttpFilters() []*envoyhttp.HttpFilter {
	var filtersByStage []stagedFilter
	for _, plug := range t.plugins {
		filterPlugin, ok := plug.(plugins.FilterPlugin)
		if !ok {
			continue
		}
		params := &plugins.FilterPluginParams{}
		stagedFilters := filterPlugin.HttpFilters(params)
		for _, httpFilter := range stagedFilters {
			if httpFilter.HttpFilter == nil {
				log.Warnf("plugin implements HttpFilters() but returned nil")
				continue
			}
			filtersByStage = append(filtersByStage, stagedFilter{
				filter: httpFilter.HttpFilter,
				stage:  httpFilter.Stage,
			})
		}
	}

	// sort filters by stage
	httpFilters := sortFilters(filtersByStage)
	httpFilters = append(httpFilters, &envoyhttp.HttpFilter{Name: routerFilter})
	return httpFilters
}

func (t *Translator) constructFilters(routeConfigName string, httpFilters []*envoyhttp.HttpFilter) ([]envoylistener.Filter, error) {
	httpConnMgr := &envoyhttp.HttpConnectionManager{
		CodecType:  envoyhttp.AUTO,
		StatPrefix: "http",
		RouteSpecifier: &envoyhttp.HttpConnectionManager_Rds{
			Rds: &envoyhttp.Rds{
				ConfigSource: envoycore.ConfigSource{
					ConfigSourceSpecifier: &envoycore.ConfigSource_Ads{
						Ads: &envoycore.AggregatedConfigSource{},
					},
				},
				RouteConfigName: routeConfigName,
			},
		},
		HttpFilters: httpFilters,
	}

	httpConnMgrCfg, err := envoyutil.MessageToStruct(httpConnMgr)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert proto message to struct")
	}
	return []envoylistener.Filter{
		{
			Name:   connMgrFilter,
			Config: httpConnMgrCfg,
		},
	}, nil
}

func sortFilters(filters []stagedFilter) []*envoyhttp.HttpFilter {
	// sort them first by stage, then by name.
	less := func(i, j int) bool {
		filteri := filters[i]
		filterj := filters[j]
		if filteri.stage != filterj.stage {
			return filteri.stage < filterj.stage
		}
		return filteri.filter.Name < filterj.filter.Name
	}
	sort.SliceStable(filters, less)

	var sortedFilters []*envoyhttp.HttpFilter
	for _, filter := range filters {
		sortedFilters = append(sortedFilters, filter.filter)
	}

	return sortedFilters
}
