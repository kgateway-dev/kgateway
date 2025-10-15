//go:build e2e

package tests

import (
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/waypoint"
)

func WaypointSuiteRunner() e2e.SuiteRunner {
	kubeGatewaySuiteRunner := e2e.NewSuiteRunner(false)
	kubeGatewaySuiteRunner.Register("Waypoint", waypoint.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("WaypointIngress", waypoint.NewIngressTestingSuite)
	return kubeGatewaySuiteRunner
}
