package translator

import (
	"context"
	"fmt"
	"strings"

	"github.com/solo-io/solo-kit/pkg/utils/contextutils"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/reporter"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

const GatewayProxyName = "gateway-proxy"

func Translate(ctx context.Context, namespace string, snap *v1.ApiSnapshot) (*gloov1.Proxy, reporter.ResourceErrors) {
	logger := contextutils.LoggerFrom(ctx)

	resourceErrs := make(reporter.ResourceErrors)
	resourceErrs.Accept(snap.Gateways.List().AsInputResources()...)
	resourceErrs.Accept(snap.VirtualServices.List().AsInputResources()...)
	if len(snap.Gateways.List()) == 0 {
		logger.Debugf("%v had no gateways", snap.Hash())
		return nil, resourceErrs
	}
	if len(snap.VirtualServices.List()) == 0 {
		logger.Debugf("%v had no virtual services", snap.Hash())
		return nil, resourceErrs
	}
	validateGateways(snap.Gateways.List(), resourceErrs)
	validateVirtualServices(snap.VirtualServices.List(), resourceErrs)
	var listeners []*gloov1.Listener
	for _, gateway := range snap.Gateways.List() {
		listener := desiredListener(gateway, snap.VirtualServices.List(), resourceErrs)
		listeners = append(listeners, listener)
	}
	return &gloov1.Proxy{
		Metadata: core.Metadata{
			Name:      GatewayProxyName,
			Namespace: namespace,
		},
		Listeners: listeners,
	}, resourceErrs
}

func joinGatewayNames(gateways v1.GatewayList) string {
	var names []string
	for _, gw := range gateways {
		names = append(names, gw.Metadata.Name)
	}
	return strings.Join(names, ".")
}

// TODO(ilackarms): implement validation func
func validateGateways(gateways v1.GatewayList, resourceErrs reporter.ResourceErrors) {

}

func validateVirtualServices(virtualServices v1.VirtualServiceList, resourceErrs reporter.ResourceErrors) {

}

func desiredListener(gateway *v1.Gateway, virtualServices v1.VirtualServiceList, resourceErrs reporter.ResourceErrors) *gloov1.Listener {
	virtualServicesForGateway := gateway.VirtualServices
	// add all virtual services if empty
	if len(gateway.VirtualServices) == 0 {
		for _, virtualService := range virtualServices {
			virtualServicesForGateway = append(virtualServicesForGateway, core.ResourceRef{
				Name:      virtualService.GetMetadata().Name,
				Namespace: virtualService.GetMetadata().Namespace,
			})
		}
	}

	var (
		virtualHosts []*gloov1.VirtualHost
		sslConfigs   []*gloov1.SslConfig
	)

	for _, ref := range virtualServicesForGateway {
		// virtual service must live in the same namespace as gateway
		virtualService, err := virtualServices.Find(ref.Strings())
		if err != nil {
			resourceErrs.AddError(gateway, err)
			continue
		}
		if virtualService.VirtualHost == nil {
			virtualService.VirtualHost = &gloov1.VirtualHost{}
		}
		virtualService.VirtualHost.Name = fmt.Sprintf("%v.%v", ref.Namespace, ref.Name)
		virtualHosts = append(virtualHosts, virtualService.VirtualHost)
	}
	return &gloov1.Listener{
		Name:        gateway.Metadata.Name,
		BindAddress: gateway.BindAddress,
		BindPort:    gateway.BindPort,
		ListenerType: &gloov1.Listener_HttpListener{
			HttpListener: &gloov1.HttpListener{
				VirtualHosts: virtualHosts,
			},
		},
		SslConfiguations: sslConfigs,
	}
}
