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
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/metrics"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	reports "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

var logger = logging.New("translator/gateway")

type TranslatorConfig struct {
	ListenerTranslatorConfig listener.ListenerTranslatorConfig
}

func NewTranslator(queries query.GatewayQueries, settings TranslatorConfig) extensionsplug.KGwTranslator {
	return &translator{
		queries:  queries,
		settings: settings,
	}
}

type translator struct {
	queries  query.GatewayQueries
	settings TranslatorConfig
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

	var rErr error

	finishMetrics := metrics.CollectTranslationMetrics(metrics.TranslatorMetricLabels{
		Name:       gateway.Name,
		Namespace:  gateway.Namespace,
		Translator: "TranslateGateway",
	})
	defer func() {
		finishMetrics(rErr)
	}()

	routesForGw, err := t.queries.GetRoutesForGateway(kctx, ctx, gateway)
	if err != nil {
		logger.Error("failed to get routes for gateway", "namespace", gateway.Namespace, "name", gateway.Name, "error", err)
		// TODO: decide how/if to report this error on Gateway
		// reporter.Gateway(gateway).Err(err.Error())
		rErr = err

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

	setAttachedRoutes(gateway, routesForGw, reporter)

	listeners := listener.TranslateListeners(
		kctx,
		ctx,
		t.queries,
		gateway,
		routesForGw,
		reporter,
		t.settings.ListenerTranslatorConfig,
	)

	return &ir.GatewayIR{
		SourceObject:                  gateway,
		Listeners:                     listeners,
		AttachedPolicies:              gateway.AttachedListenerPolicies,
		AttachedHttpPolicies:          gateway.AttachedHttpPolicies,
		PerConnectionBufferLimitBytes: gateway.PerConnectionBufferLimitBytes,
	}
}

func setAttachedRoutes(gateway *ir.Gateway, routesForGw *query.RoutesForGwResult, reporter reports.Reporter) {
	for _, listener := range gateway.Listeners {
		parentReporter := listener.GetParentReporter(reporter)

		availRoutes := 0
		if res := routesForGw.GetListenerResult(listener.Parent, string(listener.Name)); res != nil {
			// TODO we've never checked if the ListenerResult has an error.. is it already on RouteErrors?
			availRoutes = len(res.Routes)
		}
		parentReporter.Listener(&listener.Listener).SetAttachedRoutes(uint(availRoutes))
	}
}
