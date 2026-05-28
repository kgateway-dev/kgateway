package irtranslator

import (
	"errors"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const (
	routingSubsystem = "routing"

	gatewayLabel          = "gateway"
	gatewayNamespaceLabel = "gateway_namespace"
	errorTypeLabel        = "error_type"
	portLabel             = "port"
	namespaceLabel        = "namespace"

	ErrTypeRefNotFound = "ref_not_found"
	ErrTypeInvalidCfg  = "invalid_config"
)

var domainsPerListener = metrics.NewGauge(
	metrics.GaugeOpts{
		Subsystem: routingSubsystem,
		Name:      "domains",
		Help:      "Number of domains per listener",
	},
	[]string{namespaceLabel, gatewayLabel, portLabel},
)

var routeReplacementsTotal = metrics.NewCounter(
	metrics.CounterOpts{
		Subsystem: routingSubsystem,
		Name:      "replacements_total",
		Help:      "Number of routes, virtual hosts, or route configurations replaced with a synthetic 500 direct response due to invalid configuration detected during translation.",
	},
	[]string{gatewayNamespaceLabel, gatewayLabel, errorTypeLabel},
)

// domainsPerListenerMetricLabels is used as an argument to SetDomainPerListener
type domainsPerListenerMetricLabels struct {
	Namespace   string
	GatewayName string
	Port        string
}

// toMetricsLabels converts DomainPerListenerLabels to a slice of metrics.Labels.
func (r domainsPerListenerMetricLabels) toMetricsLabels() []metrics.Label {
	return []metrics.Label{
		{Name: namespaceLabel, Value: r.Namespace},
		{Name: gatewayLabel, Value: r.GatewayName},
		{Name: portLabel, Value: r.Port},
	}
}

// setDomainsPerListener sets the number of domains per listener gauge metric.
func setDomainsPerListener(labels domainsPerListenerMetricLabels, domains int) {
	if !metrics.Active() {
		return
	}

	domainsPerListener.Set(float64(domains), labels.toMetricsLabels()...)
}

type routeReplacementMetricLabels struct {
	GatewayNamespace string
	GatewayName      string
	ErrorType        string
}

func (r routeReplacementMetricLabels) toMetricsLabels() []metrics.Label {
	return []metrics.Label{
		{Name: gatewayNamespaceLabel, Value: r.GatewayNamespace},
		{Name: gatewayLabel, Value: r.GatewayName},
		{Name: errorTypeLabel, Value: r.ErrorType},
	}
}

func incRouteReplacementMetric(gw ir.GatewayIR, err error) {
	if !metrics.Active() {
		return
	}
	routeReplacementsTotal.Inc(routeReplacementMetricLabels{
		GatewayNamespace: gw.SourceObject.GetNamespace(),
		GatewayName:      gw.SourceObject.GetName(),
		ErrorType:        classifyErr(err),
	}.toMetricsLabels()...)
}

// classifyErr returns the error_type label for err. With errors.Join, the first
// matching leaf wins, so the order in which callers join errors is significant.
// Reaching this function means a replacement was emitted, so unrecognized errors
// fall back to invalid_config rather than a distinct "unknown" bucket.
func classifyErr(joinedErr error) string {
	for _, err := range ir.FlattenJoinedErr(joinedErr) {
		switch {
		case errors.Is(err, krtcollections.ErrPolicyNotFound),
			errors.Is(err, krtcollections.ErrMissingReferenceGrant),
			errors.Is(err, pluginutils.ErrGatewayExtensionNotFound):
			return ErrTypeRefNotFound
		case errors.Is(err, ErrInvalidMatcher),
			errors.Is(err, ErrInvalidRoute),
			errors.Is(err, krtcollections.ErrUnknownBackendKind):
			return ErrTypeInvalidCfg
		}
		var extTypeErr *pluginutils.ExtensionTypeError
		var polErr *ir.PolicyError
		if errors.As(err, &extTypeErr) || errors.As(err, &polErr) {
			return ErrTypeInvalidCfg
		}
	}
	return ErrTypeInvalidCfg
}
