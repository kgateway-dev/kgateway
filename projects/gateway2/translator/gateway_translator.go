package translator

import (
	"context"
	"fmt"

	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/registry"

	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	"github.com/solo-io/gloo/projects/gateway2/translator/listener"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// K8sGwTranslator This translator Translates K8s Gateway resources into Gloo Edge Proxies.
type K8sGwTranslator interface {
	// TranslateProxy This function is called by the reconciler when a K8s Gateway resource is created or updated.
	// It returns an instance of the Gloo Edge Proxy resource, that should configure a target Gloo Edge Proxy workload.
	// A null return value indicates the K8s Gateway resource failed to translate into a Gloo Edge Proxy. The error will be reported on the provided reporter.
	TranslateProxy(
		ctx context.Context,
		gateway *gwv1.Gateway,
		writeNamespace string,
		reporter reports.Reporter,
	) *v1.Proxy
}

func NewTranslator(queries query.GatewayQueries, pluginRegistry registry.PluginRegistry) K8sGwTranslator {
	return &translator{
		pluginRegistry: pluginRegistry,
		queries:        queries,
	}
}

type translator struct {
	pluginRegistry registry.PluginRegistry
	queries        query.GatewayQueries
}

func (t *translator) TranslateProxy(
	ctx context.Context,
	gateway *gwv1.Gateway,
	writeNamespace string,
	reporter reports.Reporter,
) *v1.Proxy {
	routesForGw, err := t.queries.GetRoutesForGateway(ctx, gateway)
	if err != nil {
		// TODO(ilackarms): fill in the specific error / validation
		// reporter.Gateway(gateway).Err(err.Error())
		return nil
	}
	for _, rErr := range routesForGw.RouteErrors {
		reporter.Route(&rErr.Route).ParentRef(&rErr.ParentRef).SetCondition(reports.HTTPRouteCondition{
			Type:   gwv1.RouteConditionAccepted,
			Status: metav1.ConditionFalse,
			Reason: rErr.Error.Reason,
			// TODO message
		})
	}

	for _, listener := range gateway.Spec.Listeners {
		availRoutes := 0
		if res, ok := routesForGw.ListenerResults[string(listener.Name)]; ok {
			// TODO we've never checked if the ListenerResult has an error.. is it already on RouteErrors?
			availRoutes = len(res.Routes)
		}
		reporter.Gateway(gateway).Listener(&listener).SetAttachedRoutes(uint(availRoutes))
	}

	listeners := listener.TranslateListeners(
		ctx,
		t.queries,
		t.pluginRegistry,
		gateway,
		routesForGw,
		reporter,
	)

	return &v1.Proxy{
		Metadata:  proxyMetadata(gateway, writeNamespace),
		Listeners: listeners,
	}
}

func proxyMetadata(gateway *gwv1.Gateway, writeNamespace string) *core.Metadata {
	return &core.Metadata{
		// Add the gateway name to the proxy name to ensure uniqueness of proxies
		Name: fmt.Sprintf("%s-%s", gateway.GetNamespace(), gateway.GetName()),

		// This needs to match the writeNamespace because the proxyClient will only look at namespaces in the whitelisted namespace list
		Namespace: writeNamespace,

		// All proxies are created in the writeNamespace (ie. gloo-system).
		// We apply a label to maintain a reference to where the originating Gateway was defined
		Labels: map[string]string{
			// the proxy type key/value must stay in sync with the one defined in projects/gateway2/proxy_syncer/proxy_syncer.go
			utils.ProxyTypeKey:        utils.GatewayApiProxyValue,
			utils.GatewayNamespaceKey: gateway.GetNamespace(),
		},
	}
}
