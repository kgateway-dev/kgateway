package translator

import (
	"fmt"

	errors "github.com/rotisserie/eris"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/hashutils"
)

var _ ListenerTranslator = new(HybridTranslator)

const HybridTranslatorName = "hybrid"

var (
	EmptyHybridGatewayMessage = "hybrid gateway does not have any populated matched gateways"
)

type HybridTranslator struct {
	VirtualServiceTranslator *VirtualServiceTranslator
	TcpTranslator            *TcpTranslator
}

func (t *HybridTranslator) ComputeListener(params Params, proxyName string, gateway *v1.Gateway) *gloov1.Listener {
	hybridGateway := gateway.GetHybridGateway()
	if hybridGateway == nil {
		return nil
	}

	var hybridListener *gloov1.HybridListener

	matchedGateways := hybridGateway.GetMatchedGateways()
	delegatedHttpGateways := hybridGateway.GetDelegatedHttpGateways()
	delegatedTcpGateways := hybridGateway.GetDelegatedTcpGateways()
	if matchedGateways == nil && delegatedHttpGateways == nil && delegatedTcpGateways == nil {
		return nil
	}

	// MatchedGateways take precedence over DelegatedHttpGateways/DelegatedTcpGateways
	if matchedGateways != nil {
		hybridListener = t.computeHybridListenerFromMatchedGateways(params, proxyName, gateway, matchedGateways)
		if len(hybridListener.GetMatchedListeners()) == 0 {
			// matched gateways are define inline, and therefore if they don't produce
			// any matched listeners, there is an error on the gateway resource
			params.reports.AddError(gateway, errors.New(EmptyHybridGatewayMessage))
			return nil
		}
	} else {
		// DelegatedHttpGateways/DelegatedTcpGateways are only processed if there are no MatchedGateways defined
		hybridListener = t.computeHybridListenerFromDelegatedGateways(params, proxyName, gateway, delegatedHttpGateways, delegatedTcpGateways)
		if len(hybridListener.GetMatchedListeners()) == 0 {
			// missing refs should only result in a warning
			// this allows resources to be applied asynchronously if the validation webhook is configured to allow warnings
			params.reports.AddWarning(gateway, EmptyHybridGatewayMessage)
			return nil
		}
	}

	listener := makeListener(gateway)
	listener.ListenerType = &gloov1.Listener_HybridListener{
		HybridListener: hybridListener,
	}

	if err := appendSource(listener, gateway); err != nil {
		// should never happen
		params.reports.AddError(gateway, err)
	}

	return listener
}

func (t *HybridTranslator) computeHybridListenerFromMatchedGateways(
	params Params,
	proxyName string,
	gateway *v1.Gateway,
	matchedGateways []*v1.MatchedGateway,
) *gloov1.HybridListener {
	snap := params.snapshot
	hybridListener := &gloov1.HybridListener{}
	loggedError := false

	for _, matchedGateway := range matchedGateways {
		matchedListener := &gloov1.MatchedListener{
			Matcher: &gloov1.Matcher{
				SslConfig:          matchedGateway.GetMatcher().GetSslConfig(),
				SourcePrefixRanges: matchedGateway.GetMatcher().GetSourcePrefixRanges(),
			},
		}

		switch gt := matchedGateway.GetGatewayType().(type) {
		case *v1.MatchedGateway_HttpGateway:
			if len(snap.VirtualServices) == 0 {
				if !loggedError {
					snapHash := hashutils.MustHash(snap)
					contextutils.LoggerFrom(params.ctx).Debugf("%v had no virtual services", snapHash)
					loggedError = true // only log no virtual service error once
				}
				continue
			}

			httpGateway := matchedGateway.GetHttpGateway()
			sslGateway := matchedGateway.GetMatcher().GetSslConfig() != nil
			virtualServices := getVirtualServicesForHttpGateway(params, gateway, httpGateway, sslGateway)

			matchedListener.ListenerType = &gloov1.MatchedListener_HttpListener{
				HttpListener: &gloov1.HttpListener{
					VirtualHosts: t.VirtualServiceTranslator.ComputeVirtualHosts(params, gateway, virtualServices, proxyName),
					Options:      httpGateway.GetOptions(),
				},
			}

			if sslGateway {
				virtualServices.Each(func(vs *v1.VirtualService) {
					matchedListener.SslConfigurations = append(matchedListener.GetSslConfigurations(), vs.GetSslConfig())
				})
			}

		case *v1.MatchedGateway_TcpGateway:

			matchedListener.GetMatcher().PassthroughCipherSuites = matchedGateway.GetMatcher().GetPassthroughCipherSuites()
			matchedListener.ListenerType = &gloov1.MatchedListener_TcpListener{
				TcpListener: t.TcpTranslator.ComputeTcpListener(gt.TcpGateway),
			}

			validateTcpHosts(params, gateway, gt.TcpGateway, matchedGateway.GetMatcher().GetSslConfig())

		}

		hybridListener.MatchedListeners = append(hybridListener.GetMatchedListeners(), matchedListener)
	}

	return hybridListener
}

func validateTcpHosts(params Params, gateway *v1.Gateway, matchedgateway *v1.TcpGateway, tcpSSL *ssl.SslConfig) {
	// mimick validateVirtualServiceDomains that httpgateways go through
	if tcpSSL == nil || gateway == nil || matchedgateway == nil {
		return
	}
	matcherSNIDomains := tcpSSL.GetSniDomains()
	domainMap := make(map[string]struct{})
	for _, msd := range matcherSNIDomains {

		domainMap[msd] = struct{}{}
	}
	conflictingHostDomains := make([]string, 0)
	for _, host := range matchedgateway.GetTcpHosts() {

		domains := host.GetSslConfig().GetSniDomains()
		for _, d := range domains {
			_, ok := domainMap[d]
			if !ok {
				if len(matcherSNIDomains) > 0 {
					conflictingHostDomains = append(conflictingHostDomains, fmt.Sprintf("%s:%s", host.GetName(), d))
				}
			}
		}
	}

	// mimick the behavior for http gateways and virtual hosts
	// if we specify matcher info then dont allow conflicting sni domains otherwise dont worry about it
	if len(conflictingHostDomains) > 0 {
		params.reports.AddError(gateway, errors.Errorf("gateway has conflicting sni domains %v", conflictingHostDomains))
	}
}

func (t *HybridTranslator) computeHybridListenerFromDelegatedGateways(
	params Params,
	proxyName string,
	gateway *v1.Gateway,
	delegatedHttpGateway *v1.DelegatedHttpGateway,
	delegatedTcpGateway *v1.DelegatedTcpGateway,
) *gloov1.HybridListener {

	onSelectionError := func(err error) {
		params.reports.AddError(gateway, err)
	}

	httpGatewaySelector := NewHttpGatewaySelector(params.snapshot.HttpGateways)
	matchableHttpGateways := httpGatewaySelector.SelectMatchableHttpGateways(delegatedHttpGateway, onSelectionError)

	tcpGatewaySelector := NewTcpGatewaySelector(params.snapshot.TcpGateways)
	matchableTcpGateways := tcpGatewaySelector.SelectMatchableTcpGateways(delegatedTcpGateway, onSelectionError)

	if len(matchableHttpGateways) == 0 && len(matchableTcpGateways) == 0 {
		return nil
	}

	hybridListener := &gloov1.HybridListener{}

	matchableHttpGateways.Each(func(httpGw *v1.MatchableHttpGateway) {
		matchedListener := t.computeMatchedHttpListener(params, proxyName, gateway, httpGw)
		if matchedListener != nil {
			hybridListener.MatchedListeners = append(hybridListener.GetMatchedListeners(), matchedListener)
		}
	})
	matchableTcpGateways.Each(func(tcpGw *v1.MatchableTcpGateway) {
		matchedListener := t.computeMatchedTcpListener(params, proxyName, gateway, tcpGw)
		if matchedListener != nil {
			hybridListener.MatchedListeners = append(hybridListener.GetMatchedListeners(), matchedListener)
		}
	})

	return hybridListener
}

func (t *HybridTranslator) computeMatchedHttpListener(
	params Params,
	proxyName string,
	parentGateway *v1.Gateway,
	matchableHttpGateway *v1.MatchableHttpGateway,
) *gloov1.MatchedListener {
	sslGateway := matchableHttpGateway.GetMatcher().GetSslConfig() != nil

	// reconcile the hcm configuration that is shared by Gateway and MatchableHttpGateways
	listenerOptions := reconcileGatewayLevelHCMConfig(parentGateway, matchableHttpGateway)

	// reconcile the ssl configuration that is shared by Gateway and MatchableHttpGateways
	var sslConfig *ssl.SslConfig
	if sslGateway {
		sslConfig = reconcileGatewayLevelSslConfig(parentGateway, matchableHttpGateway)
	}

	matchedListener := &gloov1.MatchedListener{
		Matcher: &gloov1.Matcher{
			SslConfig:               sslConfig,
			SourcePrefixRanges:      matchableHttpGateway.GetMatcher().GetSourcePrefixRanges(),
			PassthroughCipherSuites: nil, // not applicable to http gateways
		},
	}

	httpGateway := matchableHttpGateway.GetHttpGateway()
	virtualServices := getVirtualServicesForHttpGateway(params, parentGateway, httpGateway, sslGateway)

	matchedListener.ListenerType = &gloov1.MatchedListener_HttpListener{
		HttpListener: &gloov1.HttpListener{
			VirtualHosts: t.VirtualServiceTranslator.ComputeVirtualHosts(params, parentGateway, virtualServices, proxyName),
			Options:      listenerOptions,
		},
	}

	if sslGateway {
		virtualServices.Each(func(vs *v1.VirtualService) {
			matchedListener.SslConfigurations = append(matchedListener.GetSslConfigurations(), vs.GetSslConfig())
		})
	}

	return matchedListener
}

func (t *HybridTranslator) computeMatchedTcpListener(
	params Params,
	proxyName string,
	parentGateway *v1.Gateway,
	matchableTcpGateway *v1.MatchableTcpGateway,

) *gloov1.MatchedListener {
	validateTcpHosts(params, parentGateway, matchableTcpGateway.GetTcpGateway(), matchableTcpGateway.GetMatcher().GetSslConfig())
	// for now the parent gateway does not provide inheritable aspects so ignore it
	return &gloov1.MatchedListener{
		Matcher: &gloov1.Matcher{
			SslConfig:               matchableTcpGateway.GetMatcher().GetSslConfig(),
			SourcePrefixRanges:      matchableTcpGateway.GetMatcher().GetSourcePrefixRanges(),
			PassthroughCipherSuites: matchableTcpGateway.GetMatcher().GetPassthroughCipherSuites(),
		},
		ListenerType: &gloov1.MatchedListener_TcpListener{
			TcpListener: t.TcpTranslator.ComputeTcpListener(matchableTcpGateway.GetTcpGateway()),
		},
	}
}
