package translator

import (
	errors "github.com/rotisserie/eris"

	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/hashutils"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
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
}

func (t *HybridTranslator) Name() string {
	return HybridTranslatorName
}

func (t *HybridTranslator) ComputeListener(params Params, proxyName string, gateway *v1.Gateway, reports reporter.ResourceReports) *gloov1.Listener {
	hybridGateway := gateway.GetHybridGateway()
	if hybridGateway == nil {
		return nil
	}

	snap := params.snapshot
	loggedError := false
	listener := makeListener(gateway)
	hybridListener := &gloov1.HybridListener{}

	for _, matchedGateway := range gateway.GetHybridGateway().GetMatchedGateways() {
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

			virtualServices := getVirtualServicesForHttpGateway(matchedGateway.GetHttpGateway(), gateway, snap.VirtualServices, reports, matchedGateway.GetMatcher().GetSslConfig() != nil)
			applyGlobalVirtualServiceSettings(params.ctx, virtualServices)
			validateVirtualServiceDomains(gateway, virtualServices, reports)
			httpListener := t.HttpTranslator.desiredHttpListener(gateway, proxyName, virtualServices, snap, reports)

			hybridListener.MatchedListeners = append(hybridListener.GetMatchedListeners(), &gloov1.MatchedListener{
				Matcher: matcher,
				ListenerType: &gloov1.MatchedListener_HttpListener{
					HttpListener: httpListener,
				},
			})
		case *v1.MatchedGateway_TcpGateway:
			// logic mirrors TcpTranslator.GenerateListeners
			hybridListener.MatchedListeners = append(hybridListener.GetMatchedListeners(), &gloov1.MatchedListener{
				Matcher: matcher,
				ListenerType: &gloov1.MatchedListener_TcpListener{
					TcpListener: &gloov1.TcpListener{
						Options:  gt.TcpGateway.GetOptions(),
						TcpHosts: gt.TcpGateway.GetTcpHosts(),
					},
				},
			})
		}
	}

	if len(hybridListener.GetMatchedListeners()) == 0 {
		reports.AddError(gateway, EmptyHybridGatewayErr())
		return nil
	}

	listener.ListenerType = &gloov1.Listener_HybridListener{
		HybridListener: hybridListener,
	}

	if err := appendSource(listener, gateway); err != nil {
		// should never happen
		reports.AddError(gateway, err)
	}

	return listener
}
