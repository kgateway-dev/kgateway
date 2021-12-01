package translator

import (
	"context"

	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/hashutils"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

type HybridTranslator struct {
	HttpTranslator *HttpTranslator
}

func (t *HybridTranslator) GenerateListeners(ctx context.Context, proxyName string, snap *v1.ApiSnapshot, filteredGateways []*v1.Gateway, reports reporter.ResourceReports) []*gloov1.Listener {
	var (
		result      []*gloov1.Listener
		loggedError bool
	)

	for _, gateway := range filteredGateways {
		if gateway.GetHybridGateway() == nil {
			continue
		}

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
						contextutils.LoggerFrom(ctx).Debugf("%v had no virtual services", snapHash)
						loggedError = true
					}
					continue // TODO: do we want the entire listener to fail or just this matched listener?
				}

				virtualServices := getVirtualServicesForHttpGateway(matchedGateway.GetHttpGateway(), gateway, snap.VirtualServices, reports, matchedGateway.GetMatcher().GetSslConfig() != nil)
				applyGlobalVirtualServiceSettings(ctx, virtualServices)
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

		listener.ListenerType = &gloov1.Listener_HybridListener{
			HybridListener: hybridListener,
		}

		if err := appendSource(listener, gateway); err != nil {
			// should never happen
			reports.AddError(gateway, err)
		}

		result = append(result, listener)
	}
	return result
}
