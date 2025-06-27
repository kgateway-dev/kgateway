package irtranslator

import (
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
)

const (
	routingSubsystem = "routing"
)

var (
	domainsPerListener = metrics.NewGauge(
		metrics.GaugeOpts{
			Subsystem: routingSubsystem,
			Name:      "domains",
			Help:      "Number of domains per listener",
		},
		[]string{"namespace", "gateway", "port"},
	)
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
		{Name: "namespace", Value: r.Namespace},
		{Name: "gateway", Value: r.GatewayName},
		{Name: "port", Value: r.Port},
	}
}

// setDomainsPerListener sets the number of domains per listener gauge metric.
func setDomainsPerListener(labels domainsPerListenerMetricLabels, domains int) {
	if !metrics.Active() {
		return
	}

	domainsPerListener.Set(float64(domains), labels.toMetricsLabels()...)
}
