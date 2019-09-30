package translator

import (
	"context"
	"fmt"
	"strings"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"

	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"

	v2 "github.com/solo-io/gloo/projects/gateway/pkg/api/v2"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

// deprecated, use defaults.GatewayProxyName
const GatewayProxyName = defaults.GatewayProxyName

type ListenerFactory interface {
	GenerateListeners(ctx context.Context, snap *v2.ApiSnapshot, filteredGateways []*v2.Gateway, reports reporter.ResourceReports) []*gloov1.Listener
}

type Translator interface {
	Translate(ctx context.Context, proxyName, namespace string, snap *v2.ApiSnapshot, filteredGateways v2.GatewayList) (*gloov1.Proxy, reporter.ResourceReports)
}

type translator struct {
	factories []ListenerFactory
}

func NewTranslator(factories []ListenerFactory) *translator {
	return &translator{factories: factories}
}

func NewDefaultTranslator() *translator {
	return NewTranslator([]ListenerFactory{&HttpTranslator{}, &TcpTranslator{}})
}

func (t *translator) Translate(ctx context.Context, proxyName, namespace string, snap *v2.ApiSnapshot, gatewaysByProxy v2.GatewayList) (*gloov1.Proxy, reporter.ResourceReports) {
	logger := contextutils.LoggerFrom(ctx)

	filteredGateways := filterGatewaysForNamespace(gatewaysByProxy, namespace)

	reports := make(reporter.ResourceReports)
	reports.Accept(snap.Gateways.AsInputResources()...)
	reports.Accept(snap.VirtualServices.AsInputResources()...)
	reports.Accept(snap.RouteTables.AsInputResources()...)
	if len(filteredGateways) == 0 {
		logger.Debugf("%v had no gateways", snap.Hash())
		return nil, reports
	}
	validateGateways(filteredGateways, snap.VirtualServices, reports)
	listeners := make([]*gloov1.Listener, 0, len(filteredGateways))
	for _, factory := range t.factories {
		listeners = append(listeners, factory.GenerateListeners(ctx, snap, filteredGateways, reports)...)
	}
	if len(listeners) == 0 {
		return nil, reports
	}
	return &gloov1.Proxy{
		Metadata: core.Metadata{
			Name:      proxyName,
			Namespace: namespace,
		},
		Listeners: listeners,
	}, reports
}

func standardListener(gateway *v2.Gateway) *gloov1.Listener {
	return &gloov1.Listener{
		Name:          gatewayName(gateway),
		BindAddress:   gateway.BindAddress,
		BindPort:      gateway.BindPort,
		Plugins:       gateway.Plugins,
		UseProxyProto: gateway.UseProxyProto,
	}
}

func gatewayName(gateway *v2.Gateway) string {
	return fmt.Sprintf("listener-%s-%d", gateway.BindAddress, gateway.BindPort)
}

func validateGateways(gateways v2.GatewayList, virtualServices v1.VirtualServiceList, reports reporter.ResourceReports) {
	bindAddresses := map[string]v2.GatewayList{}
	// if two gateway (=listener) that belong to the same proxy share the same bind address,
	// they are invalid.
	for _, gw := range gateways {
		bindAddress := fmt.Sprintf("%s:%d", gw.BindAddress, gw.BindPort)
		bindAddresses[bindAddress] = append(bindAddresses[bindAddress], gw)

		if httpGw := gw.GetHttpGateway(); httpGw != nil {
			for _, vs := range httpGw.VirtualServices {
				if _, err := virtualServices.Find(vs.Strings()); err != nil {
					reports.AddError(gw, fmt.Errorf("invalid virtual service ref %v", vs))
				}
			}
		}
	}

	for addr, gateways := range bindAddresses {
		if len(gateways) > 1 {
			for _, gw := range gateways {
				reports.AddError(gw, fmt.Errorf("bind-address %s is not unique in a proxy. gateways: %s", addr, strings.Join(gatewaysRefsToString(gateways), ",")))
			}
		}
	}
}

func gatewaysRefsToString(gateways v2.GatewayList) []string {
	var ret []string
	for _, gw := range gateways {
		ret = append(ret, gw.Metadata.Ref().Key())
	}
	return ret
}

// https://github.com/solo-io/gloo/issues/538
// Gloo should only pay attention to gateways it creates, i.e. in it's write namespace, to support
// handling multiple gloo installations
func filterGatewaysForNamespace(gateways v2.GatewayList, namespace string) v2.GatewayList {
	var filteredGateways v2.GatewayList
	for _, gateway := range gateways {
		if gateway.Metadata.Namespace == namespace {
			filteredGateways = append(filteredGateways, gateway)
		}
	}
	return filteredGateways
}
