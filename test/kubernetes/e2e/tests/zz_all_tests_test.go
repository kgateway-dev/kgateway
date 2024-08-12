package tests_test

import "testing"

var (
	allTests = map[string]func(*testing.T){
		"TestAutomtlsIstioEdgeApisGateway":              TestAutomtlsIstioEdgeApisGateway,
		"TestK8sGatewayIstioAutoMtls":                   TestK8sGatewayIstioAutoMtls,
		"TestTransformationValidationDisabled":          TestTransformationValidationDisabled,
		"TestGlooGatewayEdgeGateway":                    TestGlooGatewayEdgeGateway,
		"TestGlooctlGlooGatewayEdgeGateway":             TestGlooctlGlooGatewayEdgeGateway,
		"TestGlooctlIstioInjectEdgeApiGateway":          TestGlooctlIstioInjectEdgeApiGateway,
		"TestGlooctlK8sGateway":                         TestGlooctlK8sGateway,
		"TestGloomtlsGatewayEdgeGateway":                TestGloomtlsGatewayEdgeGateway,
		"TestHelm":                                      TestHelm,
		"TestIstioEdgeApiGateway":                       TestIstioEdgeApiGateway,
		"TestIstioRegression":                           TestIstioRegression,
		"TestK8sGatewayIstio":                           TestK8sGatewayIstio,
		"TestK8sGatewayMinimalDefaultGatewayParameters": TestK8sGatewayMinimalDefaultGatewayParameters,
		"TestK8sGatewayNoValidation":                    TestK8sGatewayNoValidation,
		"TestK8sGateway":                                TestK8sGateway,
		"TestRevisionIstioRegression":                   TestRevisionIstioRegression,
		"TestK8sGatewayIstioRevision":                   TestK8sGatewayIstioRevision,
		"TestUpgradeFromCurrentPatchLatestMinor":        TestUpgradeFromCurrentPatchLatestMinor,
		"TestUpgradeFromLastPatchPreviousMinor":         TestUpgradeFromLastPatchPreviousMinor,
		"TestValidationAlwaysAccept":                    TestValidationAlwaysAccept,
		"TestValidationStrict":                          TestValidationStrict,
	}
)

// TestGlooGateway runs all of the defined Kubernetes E2E tests for Gloo Gateway.
// By wrapping all of the tests in this parent, we ensure that we get a single
// summary at the end of the test run, and that tests fail fast and do not proceed
// to run other tests.
func TestGlooGateway(t *testing.T) {
	for testName, test := range allTests {
		t.Run(testName, test)
	}
}
