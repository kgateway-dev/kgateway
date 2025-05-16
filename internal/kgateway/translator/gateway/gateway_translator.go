package gateway

import (
	"context"

	"istio.io/istio/pkg/kube/krt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/query"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/listener"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	reports "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

var logger = logging.New("translator/gateway")

func NewTranslator(queries query.GatewayQueries) extensionsplug.KGwTranslator {
	return &translator{
		queries: queries,
	}
}

type translator struct {
	queries query.GatewayQueries
}

func (t *translator) Translate(
	kctx krt.HandlerContext,
	ctx context.Context,
	gateway *ir.Gateway,
	reporter reports.Reporter,
) *ir.GatewayIR {
	stopwatch := utils.NewTranslatorStopWatch("TranslateProxy")
	stopwatch.Start()
	defer stopwatch.Stop(ctx)

	routesForGw, err := t.queries.GetRoutesForGateway(kctx, ctx, gateway.Obj)
	if err != nil {
		logger.Error("failed to get routes for gateway", "namespace", gateway.Namespace, "name", gateway.Name, "error", err)
		// TODO: decide how/if to report this error on Gateway
		// reporter.Gateway(gateway).Err(err.Error())
		return nil
	}

	for _, rErr := range routesForGw.RouteErrors {
		reporter.Route(rErr.Route.GetSourceObject()).ParentRef(&rErr.ParentRef).SetCondition(reports.RouteCondition{
			Type:   gwv1.RouteConditionAccepted,
			Status: metav1.ConditionFalse,
			Reason: rErr.Error.Reason,
			// TODO message
		})
	}

	for _, listener := range gateway.Listeners {
		availRoutes := 0
		if res, ok := routesForGw.ListenerResults[string(listener.Name)]; ok {
			// TODO we've never checked if the ListenerResult has an error.. is it already on RouteErrors?
			availRoutes = len(res.Routes)
		}
		reporter.Gateway(gateway.Obj).ListenerName(string(listener.Name)).SetAttachedRoutes(uint(availRoutes))
	}

	listeners := listener.TranslateListeners(
		kctx,
		ctx,
		t.queries,
		gateway,
		routesForGw,
		reporter,
	)

	return &ir.GatewayIR{
		SourceObject:         gateway.Obj,
		Listeners:            listeners,
		AttachedPolicies:     gateway.AttachedListenerPolicies,
		AttachedHttpPolicies: gateway.AttachedHttpPolicies,
	}
}
