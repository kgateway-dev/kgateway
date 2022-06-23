package translator

import (
	"errors"

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

	default:
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
	var httpFilterChains []*gloov1.AggregateListener_HttpFilterChain
	availableHttpListenerOptionsByName := make(map[string]*gloov1.HttpListenerOptions)
	availableVirtualHostsByName := make(map[string]*gloov1.VirtualHost)

	sslGateway := gateway.GetSsl()
	httpGateway := gateway.GetHttpGateway()
	virtualServices := getVirtualServicesForHttpGateway(params, gateway, httpGateway, sslGateway)

	if gateway.GetSsl() {
		// for an ssl gateway, create an HttpFilterChain per unique SslConfig
		virtualServicesBySslConfig := groupVirtualServicesBySslConfig(virtualServices)
		for sslConfig, virtualServiceList := range virtualServicesBySslConfig {
			httpListener := &gloov1.HttpListener{
				VirtualHosts: a.VirtualServiceTranslator.ComputeVirtualHosts(params, gateway, virtualServiceList, proxyName),
				Options:      httpGateway.GetOptions(),
			}

			httpOptionsRef := gateway.GetMetadata().Ref().Key()
			httpOptions := httpListener.GetOptions()
			availableHttpListenerOptionsByName[httpOptionsRef] = httpOptions

			var virtualHostRefs []string
			for _, virtualHost := range httpListener.GetVirtualHosts() {
				virtualHostRef := virtualHost.GetName()
				virtualHostRefs = append(virtualHostRefs, virtualHostRef)
				availableVirtualHostsByName[virtualHostRef] = virtualHost
			}

			httpFilterChain := &gloov1.AggregateListener_HttpFilterChain{
				Matcher: &gloov1.Matcher{
					SslConfig:          sslConfig,
					SourcePrefixRanges: nil, // not supported for HttpListener
				},
				HttpOptionsRef:  httpOptionsRef,
				VirtualHostRefs: virtualHostRefs,
			}
			httpFilterChains = append(httpFilterChains, httpFilterChain)
		}
	} else {
		// for a non-ssl gateway, create a single HttpFilterChain
		httpListener := &gloov1.HttpListener{
			VirtualHosts: a.VirtualServiceTranslator.ComputeVirtualHosts(params, gateway, virtualServices, proxyName),
			Options:      httpGateway.GetOptions(),
		}

		httpOptionsRef := gateway.GetMetadata().Ref().Key()
		httpOptions := httpListener.GetOptions()
		availableHttpListenerOptionsByName[httpOptionsRef] = httpOptions

		var virtualHostRefs []string
		for _, virtualHost := range httpListener.GetVirtualHosts() {
			virtualHostRef := virtualHost.GetName()
			virtualHostRefs = append(virtualHostRefs, virtualHostRef)
			availableVirtualHostsByName[virtualHostRef] = virtualHost
		}

		httpFilterChain := &gloov1.AggregateListener_HttpFilterChain{
			Matcher:         nil,
			HttpOptionsRef:  httpOptionsRef,
			VirtualHostRefs: virtualHostRefs,
		}
		httpFilterChains = append(httpFilterChains, httpFilterChain)
	}

	return &gloov1.AggregateListener{
		HttpResources: &gloov1.AggregateListener_HttpResources{
			VirtualHosts: availableVirtualHostsByName,
			HttpOptions:  availableHttpListenerOptionsByName,
		},
		HttpFilterChains: httpFilterChains,
	}
}

func (a *AggregateTranslator) computeAggregateListenerForHybridGateway(params Params, proxyName string, gateway *v1.Gateway) *gloov1.AggregateListener {
	params.reports.AddError(gateway, errors.New("not implemented"))
	return nil
}
