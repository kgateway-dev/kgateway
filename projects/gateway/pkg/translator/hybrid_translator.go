package translator

import (
	errors "github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/hashutils"
)

var _ ListenerTranslator = new(HybridTranslator)

const HybridTranslatorName = "hybrid"

var (
	EmptyHybridGatewayMessage = "hybrid gateway does not have any populated matched gateways"
)

type HybridTranslator struct {
	HttpTranslator *HttpTranslator
	TcpTranslator  *TcpTranslator
}

func (t *HybridTranslator) Name() string {
	return HybridTranslatorName
}

func (t *HybridTranslator) ComputeListener(params Params, proxyName string, gateway *v1.Gateway) *gloov1.Listener {
	hybridGateway := gateway.GetHybridGateway()
	if hybridGateway == nil {
		return nil
	}

	var hybridListener *gloov1.HybridListener

	matchedGateways := hybridGateway.GetMatchedGateways()
	delegatedGateways := hybridGateway.GetDelegatedHttpGateways()
	if matchedGateways == nil && delegatedGateways == nil {
		return nil
	}

	// MatchedGateways take precedence over DelegatedHttpGateways
	if matchedGateways != nil {
		hybridListener = t.computeHybridListenerFromMatchedGateways(params, proxyName, gateway, matchedGateways)
		if len(hybridListener.GetMatchedListeners()) == 0 {
			// matched gateways are define inline, and therefore if they don't produce
			// any matched listeners, there is an error on the gateway resource
			params.reports.AddError(gateway, errors.New(EmptyHybridGatewayMessage))
			return nil
		}
	} else {
		// DelegatedHttpGateways is only processed if there are no MatchedGateways defined
		hybridListener = t.computeHybridListenerFromDelegatedGateway(params, proxyName, gateway, delegatedGateways)
		if len(hybridListener.GetMatchedListeners()) == 0 {
			// missing refs should only result in a warning
			// this allows resources to be applied asynchronously
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
				HttpListener: t.HttpTranslator.ComputeHttpListener(params, gateway, httpGateway, virtualServices, proxyName),
			}

			if sslGateway {
				virtualServices.Each(func(vs *v1.VirtualService) {
					matchedListener.SslConfigurations = append(matchedListener.GetSslConfigurations(), vs.GetSslConfig())
				})
			}

		case *v1.MatchedGateway_TcpGateway:
			matchedListener.ListenerType = &gloov1.MatchedListener_TcpListener{
				TcpListener: t.TcpTranslator.ComputeTcpListener(gt.TcpGateway),
			}
		}

		hybridListener.MatchedListeners = append(hybridListener.GetMatchedListeners(), matchedListener)
	}

	return hybridListener
}

func (t *HybridTranslator) computeHybridListenerFromDelegatedGateway(
	params Params,
	proxyName string,
	gateway *v1.Gateway,
	delegatedGateway *v1.DelegatedHttpGateway,
) *gloov1.HybridListener {
	httpGatewaySelector := NewHttpGatewaySelector(params.snapshot.HttpGateways)
	onSelectionError := func(err error) {
		params.reports.AddError(gateway, err)
	}
	matchableHttpGateways := httpGatewaySelector.SelectMatchableHttpGateways(delegatedGateway, onSelectionError)
	if len(matchableHttpGateways) == 0 {
		return nil
	}

	hybridListener := &gloov1.HybridListener{}

	matchableHttpGateways.Each(func(element *v1.MatchableHttpGateway) {
		matchedListener := t.computeMatchedListener(params, proxyName, gateway, element)
		if matchedListener != nil {
			hybridListener.MatchedListeners = append(hybridListener.GetMatchedListeners(), matchedListener)
		}
	})

	return hybridListener
}

func (t *HybridTranslator) computeMatchedListener(
	params Params,
	proxyName string,
	parentGateway *v1.Gateway,
	matchableHttpGateway *v1.MatchableHttpGateway,
) *gloov1.MatchedListener {
	matchedListener := &gloov1.MatchedListener{
		Matcher: &gloov1.Matcher{
			SslConfig:          matchableHttpGateway.GetMatcher().GetSslConfig(),
			SourcePrefixRanges: matchableHttpGateway.GetMatcher().GetSourcePrefixRanges(),
		},
	}

	httpGateway := matchableHttpGateway.GetHttpGateway()
	sslGateway := matchableHttpGateway.GetMatcher().GetSslConfig() != nil
	virtualServices := getVirtualServicesForHttpGateway(params, parentGateway, httpGateway, sslGateway)

	matchedListener.ListenerType = &gloov1.MatchedListener_HttpListener{
		HttpListener: t.HttpTranslator.ComputeHttpListener(params, parentGateway, httpGateway, virtualServices, proxyName),
	}

	if sslGateway {
		virtualServices.Each(func(vs *v1.VirtualService) {
			matchedListener.SslConfigurations = append(matchedListener.GetSslConfigurations(), vs.GetSslConfig())
		})
	}

	return matchedListener
}
