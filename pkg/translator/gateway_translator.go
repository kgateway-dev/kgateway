package translator

import (
	"context"

	"github.com/solo-io/gloo/v2/pkg/translator/httproute/filterplugins/registry"

	"github.com/solo-io/gloo/v2/pkg/query"
	"github.com/solo-io/gloo/v2/pkg/reports"
	"github.com/solo-io/gloo/v2/pkg/translator/listener"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type ProxyResult struct {
	ListenerAndRoutes []listener.ListenerAndRoutes `json:"listener_and_routes"`
}

// K8sGwTranslator This translator Translates K8s Gateway resources into Gloo Edege Proxies.
type K8sGwTranslator interface {
	// TranslateProxy This function is called by the reconciler when a K8s Gateway resource is created or updated.
	// It returns an instance of the Gloo Edge Proxy resources that should configure a target Gloo Edge Proxy.
	// A null return value indicates the K8s Gateway resource failed to translate into a Gloo Edge Proxy. The error will be reported on the provided reporter.
	TranslateProxy(
		ctx context.Context,
		gateway *gwv1.Gateway,
		queries query.GatewayQueries,
		reporter reports.Reporter,
	) *ProxyResult
}

func NewTranslator() K8sGwTranslator {
	return &translator{
		plugins: registry.NewHTTPFilterPluginRegistry(),
	}
}

type translator struct {
	plugins registry.HTTPFilterPluginRegistry
}

func (t *translator) TranslateProxy(
	ctx context.Context,
	gateway *gwv1.Gateway,
	queries query.GatewayQueries,
	reporter reports.Reporter,
) *ProxyResult {
	if !listener.ValidateGateway(gateway, queries, reporter) {
		return nil
	}

	routesForGw, err := queries.GetRoutesForGw(ctx, gateway)
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
			availRoutes = len(res.Routes)
		}
		reporter.Gateway(gateway).Listener(&listener).SetAttachedRoutes(uint(availRoutes))
	}

	listeners := listener.TranslateListeners(
		ctx,
		queries,
		t.plugins,
		gateway,
		routesForGw,
		reporter,
	)

	return &ProxyResult{ListenerAndRoutes: listeners}
}
