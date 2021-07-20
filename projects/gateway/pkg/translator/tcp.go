package translator

import (
	"context"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

type TcpTranslator struct{}

func (t *TcpTranslator) GenerateListeners(ctx context.Context, proxyName string, snap *v1.ApiSnapshot, filteredGateways []*v1.Gateway, reports reporter.ResourceReports) []*gloov1.Listener {
	var result []*gloov1.Listener
	for _, gateway := range filteredGateways {
		tcpGateway := gateway.GetTcpGateway()
		if tcpGateway == nil {
			continue
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
		result = append(result, listener)
	}
	return result
}
