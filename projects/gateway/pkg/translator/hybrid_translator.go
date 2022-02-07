package translator

import (
	errors "github.com/rotisserie/eris"

	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/hashutils"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

var _ ListenerTranslator = new(HybridTranslator)

const HybridTranslatorName = "hybrid"

var (
	EmptyHybridGatewayErr = func() error {
		return errors.Errorf("hybrid gateway does not have any populated matched gateways")
	}
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

	// MatchedGateways take precedence
	matchedGateways := hybridGateway.GetMatchedGateways()
	if len(matchedGateways) > 0 {
		hybridListener = t.ComputeHybridListenerFromMatchedGateways(params, proxyName, gateway, matchedGateways)
	}

	// DelegatedHttpGateways is only processed if there are no MatchedGateways defined
	if hybridListener == nil {
		hybridListener = t.ComputeHybridListenerFromDelegatedGateways(params, proxyName, gateway, hybridGateway.GetDelegatedHttpGateways())
	}

	if len(hybridListener.GetMatchedListeners()) == 0 {
		params.reports.AddError(gateway, EmptyHybridGatewayErr())
		return nil
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

func (t *HybridTranslator) ComputeHybridListenerFromMatchedGateways(
	params Params,
	proxyName string,
	gateway *v1.Gateway,
	matchedGateways []*v1.MatchedGateway,
) *gloov1.HybridListener {
	snap := params.snapshot
	hybridListener := &gloov1.HybridListener{}
	loggedError := false

	for _, matchedGateway := range matchedGateways {
		matcher := &gloov1.Matcher{
			SslConfig:          matchedGateway.GetMatcher().GetSslConfig(),
			SourcePrefixRanges: matchedGateway.GetMatcher().GetSourcePrefixRanges(),
		}

		switch gt := matchedGateway.GetGatewayType().(type) {
		case *v1.MatchedGateway_HttpGateway:
			// logic mirrors HttpTranslator.GenerateListeners
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

			hybridListener.MatchedListeners = append(hybridListener.GetMatchedListeners(), &gloov1.MatchedListener{
				Matcher: matcher,
				ListenerType: &gloov1.MatchedListener_HttpListener{
					HttpListener: t.HttpTranslator.ComputeHttpListener(params, gateway, httpGateway, virtualServices, proxyName),
				},
			})
		case *v1.MatchedGateway_TcpGateway:
			hybridListener.MatchedListeners = append(hybridListener.GetMatchedListeners(), &gloov1.MatchedListener{
				Matcher: matcher,
				ListenerType: &gloov1.MatchedListener_TcpListener{
					TcpListener: t.TcpTranslator.ComputeTcpListener(gt.TcpGateway),
				},
			})
		}
	}

	return nil
}

func (t *HybridTranslator) ComputeHybridListenerFromDelegatedGateways(
	params Params,
	proxyName string,
	gateway *v1.Gateway,
	delegatedGateways []*v1.DelegatedHttpGateway,
) *gloov1.HybridListener {

	return nil
}
