package translator

import (
	"context"
	"fmt"
	"strings"

	"github.com/solo-io/gloo/projects/gateway/pkg/api/v2alpha1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/reporter"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

const GatewayProxyName = "gateway-proxy"

type Translator interface {
	Translate(ctx context.Context, namespace string, snap *v2alpha1.ApiSnapshot) (*gloov1.Proxy, reporter.ResourceErrors)
}

type translator struct {
	factories []ListenerFactory
}

func NewTranslator(factories []ListenerFactory) *translator {
	return &translator{factories: factories}
}

func (t *translator) Translate(ctx context.Context, namespace string, snap *v2alpha1.ApiSnapshot) (*gloov1.Proxy, reporter.ResourceErrors) {
	logger := contextutils.LoggerFrom(ctx)

	filteredGateways := filterGatewaysForNamespace(snap.Gateways, namespace)

	resourceErrs := make(reporter.ResourceErrors)
	resourceErrs.Accept(filteredGateways.AsInputResources()...)
	resourceErrs.Accept(snap.VirtualServices.AsInputResources()...)
	if len(filteredGateways) == 0 {
		logger.Debugf("%v had no gateways", snap.Hash())
		return nil, resourceErrs
	}
	validateGateways(filteredGateways, resourceErrs)
	var listeners []*gloov1.Listener
	for _, factory := range t.factories {
		listeners = append(listeners, factory.GenerateListeners(ctx, snap, filteredGateways, resourceErrs)...)
	}
	if len(listeners) != len(filteredGateways) {
		logger.Debug("length of listeners does not match the input gateways")
	}
	var result *gloov1.Proxy
	if len(listeners) > 0 {
		result = &gloov1.Proxy{
			Metadata: core.Metadata{
				Name:      GatewayProxyName,
				Namespace: namespace,
			},
			Listeners: listeners,
		}
	}
	return result, resourceErrs
}

// https://github.com/solo-io/gloo/issues/538
// Gloo should only pay attention to gateways it creates, i.e. in it's write namespace, to support
// handling multiple gloo installations
func filterGatewaysForNamespace(gateways v2alpha1.GatewayList, namespace string) v2alpha1.GatewayList {
	var filteredGateways v2alpha1.GatewayList
	for _, gateway := range gateways {
		if gateway.Metadata.Namespace == namespace {
			filteredGateways = append(filteredGateways, gateway)
		}
	}
	return filteredGateways
}

func validateGateways(gateways v2alpha1.GatewayList, resourceErrs reporter.ResourceErrors) {
	bindAddresses := map[string]v2alpha1.GatewayList{}
	// if two gateway (=listener) that belong to the same proxy share the same bind address,
	// they are invalid.
	for _, gw := range gateways {
		bindAddress := fmt.Sprintf("%s:%d", gw.BindAddress, gw.BindPort)
		bindAddresses[bindAddress] = append(bindAddresses[bindAddress], gw)
	}

	for addr, gateways := range bindAddresses {
		if len(gateways) > 1 {
			for _, gw := range gateways {
				resourceErrs.AddError(gw, fmt.Errorf("bind-address %s is not unique in a proxy. gateways: %s", addr, strings.Join(gatewaysRefsToString(gateways), ",")))
			}
		}
	}
}

func gatewaysRefsToString(gateways v2alpha1.GatewayList) []string {
	var ret []string
	for _, gw := range gateways {
		ret = append(ret, gw.Metadata.Ref().Key())
	}
	return ret
}

func standardListener(gateway *v2alpha1.Gateway) *gloov1.Listener {
	return &gloov1.Listener{
		Name:          gatewayName(gateway),
		BindAddress:   gateway.BindAddress,
		BindPort:      gateway.BindPort,
		Plugins:       gateway.Plugins,
		UseProxyProto: gateway.UseProxyProto,
	}
}

func gatewayName(gateway *v2alpha1.Gateway) string {
	return fmt.Sprintf("listener-%s-%d", gateway.BindAddress, gateway.BindPort)
}

type ListenerFactory interface {
	GenerateListeners(ctx context.Context, snap *v2alpha1.ApiSnapshot, filteredGateways []*v2alpha1.Gateway, resourceErrs reporter.ResourceErrors) []*gloov1.Listener
}

type TcpTranslator struct{}

func (t *TcpTranslator) GenerateListeners(ctx context.Context, snap *v2alpha1.ApiSnapshot, filteredGateways []*v2alpha1.Gateway, resourceErrs reporter.ResourceErrors) []*gloov1.Listener {
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
