package translator

import (
	"context"
	"fmt"
	"strings"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/go-utils/hashutils"

	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

// deprecated, use defaults.GatewayProxyName
const GatewayProxyName = defaults.GatewayProxyName

type ListenerFactory interface {
	GenerateListeners(ctx context.Context, proxyName string, snap *v1.ApiSnapshot, filteredGateways []*v1.Gateway, reports reporter.ResourceReports) []*gloov1.Listener
}

//go:generate mockgen -destination mocks/mock_translator.go -package mocks github.com/solo-io/gloo/projects/gateway/pkg/translator Translator
type Translator interface {
	Translate(ctx context.Context, proxyName, namespace string, snap *v1.ApiSnapshot, filteredGateways v1.GatewayList) (*gloov1.Proxy, reporter.ResourceReports)
}

type translator struct {
	listenerTypes []ListenerFactory
	opts          Opts
}

func NewTranslator(factories []ListenerFactory, opts Opts) *translator {
	return &translator{
		listenerTypes: factories,
		opts:          opts,
	}
}

func NewDefaultTranslator(opts Opts) *translator {
	warnOnRouteShortCircuiting := false
	if opts.Validation != nil {
		warnOnRouteShortCircuiting = opts.Validation.WarnOnRouteShortCircuiting
	}

	return NewTranslator([]ListenerFactory{&HttpTranslator{WarnOnRouteShortCircuiting: warnOnRouteShortCircuiting}, &TcpTranslator{}}, opts)
}

func (t *translator) Translate(ctx context.Context, proxyName, namespace string, snap *v1.ApiSnapshot, gatewaysByProxy v1.GatewayList) (*gloov1.Proxy, reporter.ResourceReports) {
	logger := contextutils.LoggerFrom(ctx)

	filteredGateways := t.filterGateways(gatewaysByProxy, namespace)

	reports := make(reporter.ResourceReports)
	reports.Accept(snap.Gateways.AsInputResources()...)
	reports.Accept(snap.VirtualServices.AsInputResources()...)
	reports.Accept(snap.RouteTables.AsInputResources()...)
	if len(filteredGateways) == 0 {
		snapHash := hashutils.MustHash(snap)
		logger.Infof("%v had no gateways", snapHash)
		return nil, reports
	}
	validateGateways(filteredGateways, snap.VirtualServices, reports)
	listeners := make([]*gloov1.Listener, 0, len(filteredGateways))
	for _, listenerFactory := range t.listenerTypes {
		listeners = append(listeners, listenerFactory.GenerateListeners(ctx, proxyName, snap, filteredGateways, reports)...)
	}
	if len(listeners) == 0 {
		return nil, reports
	}
	return &gloov1.Proxy{
		Metadata: &core.Metadata{
			Name:      proxyName,
			Namespace: namespace,
		},
		Listeners: listeners,
	}, reports
}

func makeListener(gateway *v1.Gateway) *gloov1.Listener {
	return &gloov1.Listener{
		Name:          ListenerName(gateway),
		BindAddress:   gateway.BindAddress,
		BindPort:      gateway.BindPort,
		Options:       gateway.Options,
		UseProxyProto: gateway.UseProxyProto,
		RouteOptions:  gateway.RouteOptions,
	}
}

func ListenerName(gateway *v1.Gateway) string {
	return fmt.Sprintf("listener-%s-%d", gateway.BindAddress, gateway.BindPort)
}

func validateGateways(gateways v1.GatewayList, virtualServices v1.VirtualServiceList, reports reporter.ResourceReports) {
	bindAddresses := map[string]v1.GatewayList{}
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

func gatewaysRefsToString(gateways v1.GatewayList) []string {
	var ret []string
	for _, gw := range gateways {
		ret = append(ret, gw.Metadata.Ref().Key())
	}
	return ret
}

// Get the gateways that should be processed in this sync execution
func (t *translator) filterGateways(gateways v1.GatewayList, namespace string) v1.GatewayList {
	var filteredGateways v1.GatewayList
	for _, gateway := range gateways {
		// Normally, Gloo should only pay attention to Gateways it creates, i.e. in its write
		// namespace, to support handling multiple gloo installations. However, we may want to
		// configure the controller to read all the Gateway CRDs it can find.
		if t.opts.ReadGatewaysFromAllNamespaces || gateway.Metadata.Namespace == namespace {
			filteredGateways = append(filteredGateways, gateway)
		}
	}
	return filteredGateways
}
