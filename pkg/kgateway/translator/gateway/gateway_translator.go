package gateway

import (
	"context"
	"errors"
	"fmt"

	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/gatewaytls"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/query"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/listener"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	reports "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

var logger = logging.New("translator/gateway")

type TranslatorConfig struct {
	ListenerTranslatorConfig listener.ListenerTranslatorConfig
}

func NewTranslator(queries query.GatewayQueries, settings TranslatorConfig) sdk.KGwTranslator {
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

	// Resolve once during translation so invalid refs surface on Gateway status
	// and so route rewriting can point at Gateway-scoped backend clones.
	// proxy_syncer resolves again inside its KRT collection so those clones also
	// depend directly on Secret updates.
	clientCertificate, err := gatewaytls.ResolveForGateway(kctx, ctx, t.queries, gateway)
	if err != nil {
		reportGatewayBackendClientCertificateError(err, reporter.Gateway(gateway.Obj))
	} else if clientCertificate != nil {
		gatewayScopedBackends := query.BuildGatewayBackendClientCertificateVariants(routesForGw, gateway, clientCertificate)
		routesForGw = query.RewriteRoutesForBackendVariants(routesForGw, gatewayScopedBackends)
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
		parentReporter.Listener(&listener.Listener).SetAttachedRoutes(uint(availRoutes)) //nolint:gosec // G115: availRoutes is a count of routes, always non-negative
	}
}

func reportGatewayBackendClientCertificateError(err error, gatewayReporter reports.GatewayReporter) {
	reason := gwv1.GatewayReasonInvalidClientCertificateRef
	if errors.Is(err, krtcollections.ErrMissingReferenceGrant) {
		reason = gwv1.GatewayReasonRefNotPermitted
	}

	message := err.Error()
	var notFoundErr *krtcollections.NotFoundError
	if errors.As(err, &notFoundErr) {
		resourceType := notFoundErr.NotFoundObj.Kind
		if resourceType == "" {
			resourceType = "Resource"
		}
		message = fmt.Sprintf(listener.ResourceNotFoundMessageTemplate, resourceType, notFoundErr.NotFoundObj.Namespace, notFoundErr.NotFoundObj.Name)
	}

	gatewayReporter.SetCondition(reports.GatewayCondition{
		Type:    gwv1.GatewayConditionResolvedRefs,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	})
}
