package testutils

import "github.com/solo-io/gloo/projects/gateway2/reports"

func BuildReporter() (reports.Reporter, map[string]*reports.GatewayReport) {
	gr := make(map[string]*reports.GatewayReport)
	r := reports.ReportMap{
		Gateways: gr,
	}
	report := reports.NewReporter(&r)
	return report, gr
}
