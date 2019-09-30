package translator

import (
	"context"

	v2 "github.com/solo-io/gloo/projects/gateway/pkg/api/v2"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

type TcpTranslator struct{}

func (t *TcpTranslator) GenerateListeners(ctx context.Context, snap *v2.ApiSnapshot, filteredGateways []*v2.Gateway, reports reporter.ResourceReports) []*gloov1.Listener {
	var result []*gloov1.Listener
	for _, gateway := range filteredGateways {
		tcpGateway := gateway.GetTcpGateway()
		if tcpGateway == nil {
			continue
		}
		listener := standardListener(gateway)
		listener.ListenerType = &gloov1.Listener_TcpListener{
			TcpListener: &gloov1.TcpListener{
				Plugins:  tcpGateway.Plugins,
				TcpHosts: tcpGateway.Destinations,
			},
		}
		result = append(result, listener)
	}
	return result
}
