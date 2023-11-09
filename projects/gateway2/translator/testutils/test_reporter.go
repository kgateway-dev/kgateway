package testutils

import (
	"github.com/solo-io/gloo/projects/gateway2/reports"
	"k8s.io/apimachinery/pkg/types"
)

func BuildReporter() (reports.Reporter, map[types.NamespacedName]*reports.GatewayReport) {
	gr := make(map[types.NamespacedName]*reports.GatewayReport)
	r := reports.ReportMap{
		Gateways: gr,
		Routes:   make(map[types.NamespacedName]*reports.RouteReport),
	}
	report := reports.NewReporter(&r)
	return report, gr
}
