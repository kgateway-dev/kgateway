package upgrade_test

import (
	"time"

	"github.com/solo-io/gloo/test/kube2e/helper"
)

// this package follows the current strategy in enterprise upgrade tests of using curl to test validity of traffic routing.
// Curl is chosen because it is closest to our end user's experience and is a simple way to test traffic routing.
// The downside is that this may be less reliable due to string parsing rather than using actual go code to test the traffic routing.
// TODO(nfuden): Refactor to make sure its not classic api locked but also supports kubernetes gateway api.

var (
	// petstore is a simple deployment used for most of our docs to show basic traffic patterns
	petStoreHost = "petstore"

	// gateway is currently aligned with classic gateway deployment strategy
	gatewayProxyName = "gateway-proxy"
	gatewayProxyPort = 80
)

func validatePetstoreTraffic(testHelper *helper.SoloTestHelper, path string) {
	petString := "[{\"id\":1,\"name\":\"Dog\",\"status\":\"available\"},{\"id\":2,\"name\":\"Cat\",\"status\":\"pending\"}]"
	CurlAndAssertResponse(testHelper, petStoreHost, path, petString)
}

// ===================================
// Traffic Validation Curl Functions
// ===================================

func CurlAndAssertResponse(testHelper *helper.SoloTestHelper, host string, path string, expectedResponseSubstring string) {
	testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
		Protocol:          "http",
		Path:              path,
		Method:            "GET",
		Host:              host,
		Service:           gatewayProxyName,
		Port:              gatewayProxyPort,
		ConnectionTimeout: 5, // this is important, as the first curl call sometimes hangs indefinitely
		Verbose:           true,
		LogResponses:      true,
	}, expectedResponseSubstring, 1, time.Minute*1)
}
