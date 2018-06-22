package translator

import (
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
	rdsName := routeConfigName(listener)

	var networkFilters []envoylistener.Filter
	// only add the http connection manager if listener has any virtual services
	if len(inputs.Cfg.VirtualServices) > 0 {
		httpConnMgr := t.computeHttpConnectionManager(rdsName)
		// TODO (ilacakrms): add more network filters here
		networkFilters = append(networkFilters, httpConnMgr)
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
		filterChain := newSslFilterChain(certChain, privateKey, virtualService.Domains, networkFilters)
		filterChains = append(filterChains, filterChain)
	}

	if addInsecureFilterChain {
		filterChains = append(filterChains, envoylistener.FilterChain{
			Filters: networkFilters,
		})
	}

	out := &envoyapi.Listener{
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

	for _, plug := range t.plugins {
		listenerPlugin, ok := plug.(plugins.ListenerPlugin)
		if !ok {
			continue
		}
		params := &plugins.ListenerPluginParams{}
		if err := listenerPlugin.ProcessListener(params, listener, out); err != nil {
			cfgErrs.addError(role, errors.Wrap(err, "invalid listener %v"))
		}
	}

	return out
}

func newSslFilterChain(certChain, privateKey string, sniDomains []string, networkFilters []envoylistener.Filter) envoylistener.FilterChain {
	return envoylistener.FilterChain{
		FilterChainMatch: &envoylistener.FilterChainMatch{
			SniDomains: sniDomains,
		},
		Filters: networkFilters,
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
