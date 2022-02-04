package translator

import (
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

var _ ListenerTranslator = new(TcpTranslator)

const TcpTranslatorName = "tcp"

type TcpTranslator struct{}

func (t *TcpTranslator) Name() string {
	return TcpTranslatorName
}

func (t *TcpTranslator) ComputeListener(params Params, proxyName string, gateway *v1.Gateway, reports reporter.ResourceReports) *gloov1.Listener {
	tcpGateway := gateway.GetTcpGateway()
	if tcpGateway == nil {
		return nil
	}

	listener := makeListener(gateway)
	if err := appendSource(listener, gateway); err != nil {
		// should never happen
		reports.AddError(gateway, err)
	}

	listener.ListenerType = &gloov1.Listener_TcpListener{
		TcpListener: &gloov1.TcpListener{
			Options:  tcpGateway.GetOptions(),
			TcpHosts: tcpGateway.GetTcpHosts(),
		},
	}

	return listener
}
