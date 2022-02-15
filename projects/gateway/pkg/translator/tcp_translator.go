package translator

import (
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

var _ ListenerTranslator = new(TcpTranslator)

const TcpTranslatorName = "tcp"

type TcpTranslator struct{}

func (t *TcpTranslator) Name() string {
	return TcpTranslatorName
}

func (t *TcpTranslator) ComputeListener(params Params, proxyName string, gateway *v1.Gateway) *gloov1.Listener {
	tcpGateway := gateway.GetTcpGateway()
	if tcpGateway == nil {
		return nil
	}

	listener := makeListener(gateway)
	listener.ListenerType = &gloov1.Listener_TcpListener{
		TcpListener: t.ComputeTcpListener(tcpGateway),
	}

	if err := appendSource(listener, gateway); err != nil {
		// should never happen
		params.reports.AddError(gateway, err)
	}

	return listener
}

func (t *TcpTranslator) ComputeTcpListener(tcpGateway *v1.TcpGateway) *gloov1.TcpListener {
	return &gloov1.TcpListener{
		Options:  tcpGateway.GetOptions(),
		TcpHosts: tcpGateway.GetTcpHosts(),
	}
}
