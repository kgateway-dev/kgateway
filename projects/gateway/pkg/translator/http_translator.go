package translator

import (
	"github.com/solo-io/gloo/pkg/utils/settingsutil"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/hashutils"
)

var _ ListenerTranslator = new(HttpTranslator)

const HttpTranslatorName = "http"

type HttpTranslator struct {
	VirtualServiceTranslator *VirtualServiceTranslator
}

func (t *HttpTranslator) ComputeListener(params Params, proxyName string, gateway *v1.Gateway) *gloov1.Listener {
	httpGateway := gateway.GetHttpGateway()
	if httpGateway == nil {
		return nil
	}

	snap := params.snapshot
	if len(snap.VirtualServices) == 0 {
		snapHash := hashutils.MustHash(snap)
		contextutils.LoggerFrom(params.ctx).Debugf("%v had no virtual services", snapHash)
		if settingsutil.MaybeFromContext(params.ctx).GetGateway().GetTranslateEmptyGateways().GetValue() {
			contextutils.LoggerFrom(params.ctx).Debugf("but continuing since translateEmptyGateways is set", snapHash)
		} else {
			return nil

		}
	}

	sslGateway := gateway.GetSsl()
	virtualServices := getVirtualServicesForHttpGateway(params, gateway, httpGateway, sslGateway)

	listener := makeListener(gateway)
	listener.ListenerType = &gloov1.Listener_HttpListener{
		HttpListener: &gloov1.HttpListener{
			VirtualHosts: t.VirtualServiceTranslator.ComputeVirtualHosts(params, gateway, virtualServices, proxyName),
			Options:      httpGateway.GetOptions(),
		},
	}

	if sslGateway {
		virtualServices.Each(func(vs *v1.VirtualService) {
			listener.SslConfigurations = append(listener.GetSslConfigurations(), vs.GetSslConfig())
		})
	}

	if err := appendSource(listener, gateway); err != nil {
		// should never happen
		params.reports.AddError(gateway, err)
	}

	return listener
}
