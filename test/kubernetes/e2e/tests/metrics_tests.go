//go:build e2e

package tests

import (
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/metrics"
)

func KGatewayMetricsSuiteRunner() e2e.SuiteRunner {
	metricsSuiteRunner := e2e.NewSuiteRunner(false)

	metricsSuiteRunner.Register("Metrics", metrics.NewTestingSuite)

	return metricsSuiteRunner
}
