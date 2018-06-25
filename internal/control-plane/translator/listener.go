package translator

import (
	"sort"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/internal/control-plane/snapshot"
	"github.com/solo-io/gloo/pkg/plugins"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/log"
)

func (t *Translator) computeListener(role *v1.Role, listener *v1.Listener, inputs *snapshot.Cache, cfgErrs configErrors) *envoyapi.Listener {
	validateListenerPorts(role, cfgErrs)

	listenerFilters := t.computeListenerFilters(role, listener, cfgErrs)

	filterChains := createListenerFilterChains(role, inputs, listener, listenerFilters, cfgErrs)

	return &envoyapi.Listener{
		Name: listener.Name,
		Address: envoycore.Address{
			Address: &envoycore.Address_SocketAddress{
				SocketAddress: &envoycore.SocketAddress{
					Protocol: envoycore.TCP,
					Address:  listener.BindAddress,
					PortSpecifier: &envoycore.SocketAddress_PortValue{
						PortValue: listener.BindPort,
					},
					Ipv4Compat: true,
				},
			},
		},
		FilterChains: filterChains,
	}
}

// create a duplicate of the listener filter chain for each ssl cert we want to serve
// plus one if there's an insecure one
func createListenerFilterChains(role *v1.Role, inputs *snapshot.Cache, listener *v1.Listener, listenerFilters []envoylistener.Filter, cfgErrs configErrors) []envoylistener.FilterChain {
	// no filters = no filter chains
	// TODO(ilackarms): find another way to prevent the xds server from serving listeners with 0 filters
	// currently the translator does not add listeners with 0 filters to the xds snapshot
	if len(listenerFilters) == 0 {
		return nil
	}

	// if there are any insecure virtual services, need an insecure filter chain
	// that will match for them
	var addInsecureFilterChain bool
	var filterChains []envoylistener.FilterChain
	for _, virtualService := range inputs.Cfg.VirtualServices {
		if virtualService.SslConfig == nil || virtualService.SslConfig.SecretRef == "" {
			addInsecureFilterChain = true
			continue
		}
		ref := virtualService.SslConfig.SecretRef
		certChain, privateKey, err := getSslSecrets(ref, inputs.Secrets)
		if err != nil {
			log.Warnf("skipping ssl vService with invalid secrets: %v", virtualService.Name)
			continue
		}
		domains := virtualService.SslConfig.SniDomains
		if len(domains) == 0 {
			domains = virtualService.Domains
		}
		filterChain := newSslFilterChain(certChain, privateKey, domains, listenerFilters)
		filterChains = append(filterChains, filterChain)
	}

	if listener.SslConfig != nil {
		ref := listener.SslConfig.SecretRef
		certChain, privateKey, err := getSslSecrets(ref, inputs.Secrets)
		if err != nil {
			cfgErrs.addError(role, errors.Wrapf(err, "listener %v has invalid secret", listener.Name))
		}
		filterChain := newSslFilterChain(certChain, privateKey, listener.SslConfig.SniDomains, listenerFilters)
		filterChains = append(filterChains, filterChain)
	}

	// if 0 virtualservices are defined and no ssl config is provided for the listener
	// create a filter chain with no tls
	if addInsecureFilterChain || len(filterChains) == 0{
		filterChains = append(filterChains, envoylistener.FilterChain{
			Filters: listenerFilters,
		})
	}

	return filterChains
}

func validateListenerPorts(role *v1.Role, cfgErrs configErrors) {
	listenersByPort := make(map[uint32][]string)
	for _, listener := range role.Listeners {
		listenersByPort[listener.BindPort] = append(listenersByPort[listener.BindPort], listener.Name)
	}
	for port, listeners := range listenersByPort {
		if len(listeners) == 1 {
			continue
		}
		cfgErrs.addError(role, errors.Errorf("port %v is shared by listeners %v", port, listeners))
	}
}

func newSslFilterChain(certChain, privateKey string, sniDomains []string, listenerFilters []envoylistener.Filter) envoylistener.FilterChain {
	return envoylistener.FilterChain{
		FilterChainMatch: &envoylistener.FilterChainMatch{
			SniDomains: sniDomains,
		},
		Filters: listenerFilters,
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

func (t *Translator) computeListenerFilters(role *v1.Role, listener *v1.Listener, cfgErrs configErrors) []envoylistener.Filter {
	var listenerFilters []plugins.StagedListenerFilter
	for _, plug := range t.plugins {
		filterPlugin, ok := plug.(plugins.ListenerFilterPlugin)
		if !ok {
			continue
		}
		params := &plugins.ListenerFilterPluginParams{}
		stagedFilters, err := filterPlugin.ListenerFilters(params, listener)
		if err != nil {
			cfgErrs.addError(role, err)
		}
		for _, listenerFilter := range stagedFilters {
			listenerFilters = append(listenerFilters, listenerFilter)
		}
	}

	// only add the http connection manager if listener has any virtual services
	if len(listener.VirtualServices) > 0 {
		// add the http connection manager filter after all the InAuth Listener Filters
		rdsName := routeConfigName(listener)
		httpConnMgr := t.computeHttpConnectionManager(rdsName)
		listenerFilters = append(listenerFilters, plugins.StagedListenerFilter{
			ListenerFilter: httpConnMgr,
			Stage: plugins.PostInAuth,
		})
	}

	// sort filters by stage
	return sortListenerFilters(listenerFilters)
}

func sortListenerFilters(filters []plugins.StagedListenerFilter) []envoylistener.Filter {
	// sort them first by stage, then by name.
	less := func(i, j int) bool {
		filteri := filters[i]
		filterj := filters[j]
		if filteri.Stage != filterj.Stage {
			return filteri.Stage < filterj.Stage
		}
		return filteri.ListenerFilter.Name < filterj.ListenerFilter.Name
	}
	sort.SliceStable(filters, less)

	var sortedFilters []envoylistener.Filter
	for _, filter := range filters {
		sortedFilters = append(sortedFilters, filter.ListenerFilter)
	}

	return sortedFilters
}
