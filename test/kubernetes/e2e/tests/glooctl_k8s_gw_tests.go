package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/glooctl"
)

func GlooctlKubeGatewaySuiteRunner() e2e.SuiteRunner {
	kubeGatewaySuiteRunner := e2e.NewSuiteRunner(false)

	// To ensure that glooctl check works as expected in an installation with kubeGateway enabled :
	// 0. Install Edge with kubeGateway enabled (done when the test begins)
	// 1. Verify it checks kubeGateway resources
	// TODO (davidjumani) :
	// 2. Upgrade Edge with kubeGateway disabled
	// 3. Verify it does not check kubeGateway resources
	// 4. Upgrade Edge with kubeGateway enabled
	// 5. Verify it checks kubeGateway resources
	// This verifies that we are not relying on any logic / resources that can be left behind after an upgrade or when the user switches between gateway modes
	kubeGatewaySuiteRunner.Register("Check", glooctl.NewCheckSuite)
	kubeGatewaySuiteRunner.Register("CheckCrds", glooctl.NewCheckCrdsSuite)
	kubeGatewaySuiteRunner.Register("Debug", glooctl.NewDebugSuite)
	kubeGatewaySuiteRunner.Register("GetProxy", glooctl.NewGetProxySuite)

	return kubeGatewaySuiteRunner
}
