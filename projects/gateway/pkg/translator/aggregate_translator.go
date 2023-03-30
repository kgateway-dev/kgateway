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

var _ ListenerTranslator = new(AggregateTranslator)

const AggregateTranslatorName = "aggregate"

type AggregateTranslator struct {
	VirtualServiceTranslator *VirtualServiceTranslator
}

func (a *AggregateTranslator) ComputeListener(params Params, proxyName string, gateway *v1.Gateway) *gloov1.Listener {
	snap := params.snapshot

	// currently the AggregateListener only support HTTP features (and not TCP)
	// therefore, we are safe to guard against empty VirtualServices first, since all HTTP features
	// require VirtualServices to function. If TCP support is added later, this guard will need
	// to be moved
	if len(snap.VirtualServices) == 0 {
		snapHash := hashutils.MustHash(snap)
		contextutils.LoggerFrom(params.ctx).Debugf("%v had no virtual services", snapHash)
		return nil
	}

	var aggregateListener *gloov1.AggregateListener
	switch gateway.GetGatewayType().(type) {
	case *v1.Gateway_HttpGateway:
		aggregateListener = a.computeAggregateListenerForHttpGateway(params, proxyName, gateway)

	case *v1.Gateway_HybridGateway:
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
	delegatedGateways := hybridGateway.GetDelegatedHttpGateways()
	if matchedGateways == nil && delegatedGateways == nil {
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
		aggregateListener = a.computeListenerFromDelegatedGateway(params, proxyName, gateway, delegatedGateways)
		if len(aggregateListener.GetHttpFilterChains()) == 0 {
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
			params.reports.AddError(gateway, errors.New("AggregateListener does not support TCP features (yet)"))
			continue

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
	delegatedGateway *v1.DelegatedHttpGateway,
) *gloov1.AggregateListener {
	// 1. Select the HttpGateways
	httpGatewaySelector := NewHttpGatewaySelector(params.snapshot.HttpGateways)
	onSelectionError := func(err error) {
		params.reports.AddError(gateway, err)
	}
	matchableHttpGateways := httpGatewaySelector.SelectMatchableHttpGateways(delegatedGateway, onSelectionError)
	if len(matchableHttpGateways) == 0 {
		return nil
	}

	// 2. Initialize the builder, used to aggregate resources
	builder := newBuilder()

	// 3. Process each MatchableHttpGateway, which may create 1 or more distinct filter chains
	matchableHttpGateways.Each(func(element *v1.MatchableHttpGateway) {
		a.processMatchableGateway(params, proxyName, gateway, element, builder)
	})

	// 4. Build the listener
	return builder.build()
}

func (a *AggregateTranslator) processMatchableGateway(
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
	}
}
