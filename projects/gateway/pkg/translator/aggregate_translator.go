package translator

import (
	"errors"
	"strconv"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"

	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/hashutils"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

// Make sure that aggregate Translator implements our ListenerTranslator interface
var _ ListenerTranslator = new(AggregateTranslator)

const AggregateTranslatorName = "aggregate"

// AggregateTranslator is responsible for translating a Gateway into a Gloo Listener
// It attempts to use Gateways as a storage for shared filterchain options.
// Therefore it splits gateways into appropriate filterchains based on the sni options.
type AggregateTranslator struct {
	VirtualServiceTranslator *VirtualServiceTranslator
	TcpTranslator            *TcpTranslator
}

func (a *AggregateTranslator) ComputeListener(params Params, proxyName string, gateway *v1.Gateway) *gloov1.Listener {
	snap := params.snapshot

	var aggregateListener *gloov1.AggregateListener
	switch gw := gateway.GetGatewayType().(type) {
	case *v1.Gateway_HttpGateway:
		// we are safe to guard against empty VirtualServices first, since all HTTP features require VirtualServices to function.
		if len(snap.VirtualServices) == 0 {
			snapHash := hashutils.MustHash(snap)
			contextutils.LoggerFrom(params.ctx).Debugf("%v had no virtual services", snapHash)
			return nil
		}

		aggregateListener = a.computeAggregateListenerForHttpGateway(params, proxyName, gateway)

	case *v1.Gateway_HybridGateway:
		hybrid := gw.HybridGateway

		// warn early if there are no virtual services and no tcp configurations
		if len(snap.VirtualServices) == 0 {
			hasTCP := hybrid.GetDelegatedTcpGateways() != nil
			if !hasTCP && hybrid.GetMatchedGateways() != nil {
				for _, matched := range hybrid.GetMatchedGateways() {
					if matched.GetTcpGateway() != nil {
						hasTCP = true
						break
					}
				}
			}
			if !hasTCP {
				snapHash := hashutils.MustHash(snap)
				contextutils.LoggerFrom(params.ctx).Debugf("%v had no virtual services or tcp gateways", snapHash)
				return nil
			}
		}
		aggregateListener = a.computeAggregateListenerForHybridGateway(params, proxyName, gateway)
	}

	if aggregateListener == nil {
		// there are cases where a Gateway may have no configuration and we want to ensure
		// that we do not generate an empty listener
		return nil
	}

	listener := makeListener(gateway)
	listener.ListenerType = &gloov1.Listener_AggregateListener{
		AggregateListener: aggregateListener,
	}

	if err := appendSource(listener, gateway); err != nil {
		// should never happen
		params.reports.AddError(gateway, err)
	}

	return listener
}

func (a *AggregateTranslator) computeAggregateListenerForHttpGateway(params Params, proxyName string, gateway *v1.Gateway) *gloov1.AggregateListener {
	sslGateway := gateway.GetSsl()
	httpGateway := gateway.GetHttpGateway()
	virtualServices := getVirtualServicesForHttpGateway(params, gateway, httpGateway, sslGateway)

	builder := newBuilder()
	if gateway.GetSsl() {
		// for an ssl gateway, create an HttpFilterChain per unique SslConfig
		orderedSslConfigs, virtualServicesBySslConfig := GroupVirtualServicesBySslConfig(virtualServices)
		for _, vsSslConfig := range orderedSslConfigs {
			virtualServiceList := virtualServicesBySslConfig[vsSslConfig]
			virtualHosts := a.VirtualServiceTranslator.ComputeVirtualHosts(params, gateway, virtualServiceList, proxyName)
			httpOptions := httpGateway.GetOptions()
			matcher := &gloov1.Matcher{
				SslConfig:          vsSslConfig,
				SourcePrefixRanges: nil, // not supported for HttpListener
			}

			builder.addHttpFilterChain(virtualHosts, httpOptions, matcher)
		}
	} else {
		// for a non-ssl gateway, create a single HttpFilterChain
		virtualHosts := a.VirtualServiceTranslator.ComputeVirtualHosts(params, gateway, virtualServices, proxyName)
		httpOptions := httpGateway.GetOptions()

		builder.addHttpFilterChain(virtualHosts, httpOptions, nil)
	}

	return builder.build()
}

func (a *AggregateTranslator) computeAggregateListenerForHybridGateway(params Params, proxyName string, gateway *v1.Gateway) *gloov1.AggregateListener {
	hybridGateway := gateway.GetHybridGateway()

	matchedGateways := hybridGateway.GetMatchedGateways()
	delegatedHttpGateways := hybridGateway.GetDelegatedHttpGateways()
	delegatedTcpGateways := hybridGateway.GetDelegatedTcpGateways()
	if matchedGateways == nil && delegatedHttpGateways == nil && delegatedTcpGateways == nil {
		return nil
	}

	var aggregateListener *gloov1.AggregateListener

	// MatchedGateways take precedence over DelegatedHttpGateways
	if matchedGateways != nil {
		aggregateListener = a.computeListenerFromMatchedGateways(params, proxyName, gateway, matchedGateways)
		if len(aggregateListener.GetHttpFilterChains()) == 0 {
			// matched gateways are defined inline, and therefore if they don't produce
			// any filter chains, there is an error on the gateway resource
			params.reports.AddError(gateway, errors.New(EmptyHybridGatewayMessage))
			return nil
		}
	} else {
		// DelegatedHttpGateways is only processed if there are no MatchedGateways defined
		aggregateListener = a.computeListenerFromDelegatedGateway(params, proxyName, gateway, delegatedHttpGateways, delegatedTcpGateways)
		if len(aggregateListener.GetHttpFilterChains()) == 0 && len(aggregateListener.GetTcpListeners()) == 0 {
			// missing refs should only result in a warning
			// this allows resources to be applied asynchronously if the validation webhook is configured to allow warnings
			params.reports.AddWarning(gateway, EmptyHybridGatewayMessage)
			return nil
		}
	}

	return aggregateListener
}

func (a *AggregateTranslator) computeListenerFromMatchedGateways(
	params Params,
	proxyName string,
	gateway *v1.Gateway,
	matchedGateways []*v1.MatchedGateway,
) *gloov1.AggregateListener {

	builder := newBuilder()
	for _, matchedGateway := range matchedGateways {

		switch gt := matchedGateway.GetGatewayType().(type) {
		case *v1.MatchedGateway_TcpGateway:
			// for now the parent gateway does not provide inheritable aspects so ignore it
			tcpListener := &gloov1.MatchedTcpListener{
				Matcher: &gloov1.Matcher{
					SslConfig:               matchedGateway.GetMatcher().GetSslConfig(),
					SourcePrefixRanges:      matchedGateway.GetMatcher().GetSourcePrefixRanges(),
					PassthroughCipherSuites: matchedGateway.GetMatcher().GetPassthroughCipherSuites(),
				},
				TcpListener: a.TcpTranslator.ComputeTcpListener(matchedGateway.GetTcpGateway()),
			}
			if builder.tcpListeners == nil {
				builder.tcpListeners = make([]*gloov1.MatchedTcpListener, 0)
			}
			builder.tcpListeners = append(builder.tcpListeners, tcpListener)

		case *v1.MatchedGateway_HttpGateway:
			gatewaySsl := matchedGateway.GetMatcher().GetSslConfig()
			virtualServices := getVirtualServicesForHttpGateway(params, gateway, gt.HttpGateway, gatewaySsl != nil)

			if gatewaySsl != nil {
				// for an ssl gateway, create an HttpFilterChain per unique SslConfig
				orderedSslConfigs, virtualServicesBySslConfig := GroupVirtualServicesBySslConfig(virtualServices)
				for _, vsSslConfig := range orderedSslConfigs {
					virtualServiceList := virtualServicesBySslConfig[vsSslConfig]
					// SslConfig is evaluated by having the VS definition merged into the Gateway, and overriding
					// any shared fields. The Gateway is purely used to define default values.
					reconciledSslConfig := mergeSslConfig(gatewaySsl, vsSslConfig, false)
					virtualHosts := a.VirtualServiceTranslator.ComputeVirtualHosts(params, gateway, virtualServiceList, proxyName)
					httpOptions := gt.HttpGateway.GetOptions()
					matcher := &gloov1.Matcher{
						SslConfig:          reconciledSslConfig,
						SourcePrefixRanges: matchedGateway.GetMatcher().GetSourcePrefixRanges(),
					}

					builder.addHttpFilterChain(virtualHosts, httpOptions, matcher)
				}
			} else {
				// for a non-ssl gateway, create a single HttpFilterChain
				virtualHosts := a.VirtualServiceTranslator.ComputeVirtualHosts(params, gateway, virtualServices, proxyName)
				httpOptions := gt.HttpGateway.GetOptions()
				matcher := &gloov1.Matcher{
					SslConfig:          nil,
					SourcePrefixRanges: matchedGateway.GetMatcher().GetSourcePrefixRanges(),
				}

				builder.addHttpFilterChain(virtualHosts, httpOptions, matcher)
			}
		}
	}

	return builder.build()
}

func (a *AggregateTranslator) computeListenerFromDelegatedGateway(
	params Params,
	proxyName string,
	gateway *v1.Gateway,
	delegatedHttpGateway *v1.DelegatedHttpGateway,
	delegatedTcpGateway *v1.DelegatedTcpGateway,
) *gloov1.AggregateListener {

	// 1. Initialize the builder, used to aggregate resources
	builder := newBuilder()

	onSelectionError := func(err error) {
		params.reports.AddError(gateway, err)
	}

	// 2. Select the HttpGateways
	httpGatewaySelector := NewHttpGatewaySelector(params.snapshot.HttpGateways)
	matchableHttpGateways := httpGatewaySelector.SelectMatchableHttpGateways(delegatedHttpGateway, onSelectionError)

	// 3. Select the TcpGateways
	tcpGatewaySelector := NewTcpGatewaySelector(params.snapshot.TcpGateways)
	matchableTcpGateways := tcpGatewaySelector.SelectMatchableTcpGateways(delegatedTcpGateway, onSelectionError)

	// nothing to do if there are no matchable gateways so dont do anything more
	if len(matchableHttpGateways) == 0 && len(matchableTcpGateways) == 0 {
		return nil
	}

	// 4. Process each  matchable Gateway, which may create 1 or more distinct filter chains
	matchableHttpGateways.Each(func(httpGw *v1.MatchableHttpGateway) {
		a.processMatchableHttpGateway(params, proxyName, gateway, httpGw, builder)
	})
	// 5. Process each matchable tcp gateway which creates a tcp impl that by default
	// knows how to make multiple filter chains
	matchableTcpGateways.Each(func(tcpGw *v1.MatchableTcpGateway) {
		a.processMatchableTcpGateway(params, proxyName, gateway, tcpGw, builder)
	})

	// 5. Build the listener from all the accumulated resources
	return builder.build()
}

func (a *AggregateTranslator) processMatchableHttpGateway(
	params Params,
	proxyName string,
	parentGateway *v1.Gateway,
	matchableHttpGateway *v1.MatchableHttpGateway,
	builder *aggregateListenerBuilder,
) {
	sslGateway := matchableHttpGateway.GetMatcher().GetSslConfig() != nil

	// reconcile the hcm configuration that is shared by Gateway and MatchableHttpGateways
	listenerOptions := reconcileGatewayLevelHCMConfig(parentGateway, matchableHttpGateway)

	// reconcile the ssl configuration that is shared by Gateway and MatchableHttpGateways
	var sslConfig *ssl.SslConfig
	if sslGateway {
		sslConfig = reconcileGatewayLevelSslConfig(parentGateway, matchableHttpGateway)
	}

	virtualServices := getVirtualServicesForHttpGateway(params, parentGateway, matchableHttpGateway.GetHttpGateway(), sslGateway)

	if sslGateway {
		// for an ssl gateway, create an HttpFilterChain per unique SslConfig
		orderedSslConfigs, virtualServicesBySslConfig := GroupVirtualServicesBySslConfig(virtualServices)
		for _, vsSslConfig := range orderedSslConfigs {
			virtualServiceList := virtualServicesBySslConfig[vsSslConfig]
			// SslConfig is evaluated by having the VS definition merged into the Gateway, and overriding
			// any shared fields. The Gateway is purely used to define default values.
			reconciledSslConfig := mergeSslConfig(sslConfig, vsSslConfig, false)
			virtualHosts := a.VirtualServiceTranslator.ComputeVirtualHosts(params, parentGateway, virtualServiceList, proxyName)
			matcher := &gloov1.Matcher{
				SslConfig:          reconciledSslConfig,
				SourcePrefixRanges: matchableHttpGateway.GetMatcher().GetSourcePrefixRanges(),
			}

			builder.addHttpFilterChain(virtualHosts, listenerOptions, matcher)
		}
	} else {
		// for a non-ssl gateway, create a single HttpFilterChain
		virtualHosts := a.VirtualServiceTranslator.ComputeVirtualHosts(params, parentGateway, virtualServices, proxyName)
		matcher := &gloov1.Matcher{
			SslConfig:          nil,
			SourcePrefixRanges: matchableHttpGateway.GetMatcher().GetSourcePrefixRanges(),
		}

		builder.addHttpFilterChain(virtualHosts, listenerOptions, matcher)
	}
}

// processMatchableTcpGateway  from a matchedtcpGateway
// note that TCP gateways do not have nearly as much complexity as HTTP gateways
// so most other locations where we compute listeners should also be updated
// if this function is being updated.
// It is likely that this should be exported and shared at some point if we continue
// to copy it to more locations.
// For example in hybrid a similar function is called computeMatchedTcpListener
func (a *AggregateTranslator) processMatchableTcpGateway(
	params Params,
	proxyName string,
	parentGateway *v1.Gateway,
	matchableTcpGateway *v1.MatchableTcpGateway,
	builder *aggregateListenerBuilder,

) {
	validateTcpHosts(params, parentGateway, matchableTcpGateway.GetTcpGateway(), matchableTcpGateway.GetMatcher().GetSslConfig())
	// for now the parent gateway does not provide inheritable aspects so ignore it
	tcpListener := &gloov1.MatchedTcpListener{
		Matcher: &gloov1.Matcher{
			SslConfig:               matchableTcpGateway.GetMatcher().GetSslConfig(),
			SourcePrefixRanges:      matchableTcpGateway.GetMatcher().GetSourcePrefixRanges(),
			PassthroughCipherSuites: matchableTcpGateway.GetMatcher().GetPassthroughCipherSuites(),
		},
		TcpListener: a.TcpTranslator.ComputeTcpListener(matchableTcpGateway.GetTcpGateway()),
	}
	if builder.tcpListeners == nil {
		builder.tcpListeners = make([]*gloov1.MatchedTcpListener, 0)
	}
	builder.tcpListeners = append(builder.tcpListeners, tcpListener)
}

// A Gateway and MatchableHttpGateway share configuration
// reconcileGatewayLevelConfig establishes the reconciled set of options
func reconcileGatewayLevelHCMConfig(parentGateway *v1.Gateway, matchableHttpGateway *v1.MatchableHttpGateway) *gloov1.HttpListenerOptions {
	// v ---- inheritance logic ---- v
	preventChildOverrides := parentGateway.GetHybridGateway().GetDelegatedHttpGateways().GetPreventChildOverrides()

	// HcmOptions
	parentHcmOptions := parentGateway.GetHybridGateway().GetDelegatedHttpGateways().GetHttpConnectionManagerSettings()
	var childHcmOptions *hcm.HttpConnectionManagerSettings
	if matchableHttpGateway.GetHttpGateway().GetOptions() != nil {
		childHcmOptions = matchableHttpGateway.GetHttpGateway().GetOptions().GetHttpConnectionManagerSettings()
	}
	reconciledHCMSettings := mergeHCMSettings(parentHcmOptions, childHcmOptions, preventChildOverrides)

	listenerOptions := matchableHttpGateway.GetHttpGateway().GetOptions()
	if listenerOptions != nil {
		listenerOptions.HttpConnectionManagerSettings = reconciledHCMSettings
	} else {
		listenerOptions = &gloov1.HttpListenerOptions{
			HttpConnectionManagerSettings: reconciledHCMSettings,
		}
	}

	return listenerOptions
}

// A Gateway and MatchableHttpGateway share configuration
// reconcileGatewayLevelConfig establishes the reconciled set of options
func reconcileGatewayLevelSslConfig(parentGateway *v1.Gateway, matchableHttpGateway *v1.MatchableHttpGateway) *ssl.SslConfig {
	// v ---- inheritance logic ---- v
	preventChildOverrides := parentGateway.GetHybridGateway().GetDelegatedHttpGateways().GetPreventChildOverrides()

	// SslConfig
	parentSslConfig := parentGateway.GetHybridGateway().GetDelegatedHttpGateways().GetSslConfig()
	var childSslConfig *ssl.SslConfig
	if matchableHttpGateway.GetMatcher() != nil {
		childSslConfig = matchableHttpGateway.GetMatcher().GetSslConfig()
	}
	reconciledSslConfig := mergeSslConfig(parentSslConfig, childSslConfig, preventChildOverrides)

	return reconciledSslConfig
}

// aggregateListenerBuilder is a utility used to build the listener
type aggregateListenerBuilder struct {
	virtualHostsByName map[string]*gloov1.VirtualHost
	httpOptionsByName  map[string]*gloov1.HttpListenerOptions
	httpFilterChains   []*gloov1.AggregateListener_HttpFilterChain

	tcpListeners []*gloov1.MatchedTcpListener
}

func newBuilder() *aggregateListenerBuilder {
	return &aggregateListenerBuilder{
		virtualHostsByName: make(map[string]*gloov1.VirtualHost),
		httpOptionsByName:  make(map[string]*gloov1.HttpListenerOptions),
		httpFilterChains:   nil,
	}
}

func (b *aggregateListenerBuilder) addHttpFilterChain(virtualHosts []*gloov1.VirtualHost, httpOptions *gloov1.HttpListenerOptions, matcher *gloov1.Matcher) {
	// store HttpListenerOptions, indexed by a hash of the httpOptions
	httpOptionsHash, _ := httpOptions.Hash(nil)
	httpOptionsRef := strconv.Itoa(int(httpOptionsHash))
	b.httpOptionsByName[httpOptionsRef] = httpOptions

	// store VirtualHosts, indexed by the name of the VirtualHost
	var virtualHostRefs []string
	for _, virtualHost := range virtualHosts {
		virtualHostRef := virtualHost.GetName()
		virtualHostRefs = append(virtualHostRefs, virtualHostRef)
		b.virtualHostsByName[virtualHostRef] = virtualHost
	}

	httpFilterChain := &gloov1.AggregateListener_HttpFilterChain{
		Matcher:         matcher,
		HttpOptionsRef:  httpOptionsRef,
		VirtualHostRefs: virtualHostRefs,
	}
	b.httpFilterChains = append(b.httpFilterChains, httpFilterChain)
}

func (b *aggregateListenerBuilder) build() *gloov1.AggregateListener {
	return &gloov1.AggregateListener{
		HttpResources: &gloov1.AggregateListener_HttpResources{
			VirtualHosts: b.virtualHostsByName,
			HttpOptions:  b.httpOptionsByName,
		},
		HttpFilterChains: b.httpFilterChains,
		TcpListeners:     b.tcpListeners,
	}
}
