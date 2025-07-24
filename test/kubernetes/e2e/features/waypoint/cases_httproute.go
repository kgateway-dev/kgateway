package waypoint

import (
	"net/http"

	"github.com/onsi/gomega/gstruct"

	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var (
	hasHTTPRoute = matchers.HttpResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]interface{}{
			"traversed-waypoint": "true",
		},
		Body: gstruct.Ignore(),
	}

	noHTTPRoute = matchers.HttpResponse{
		StatusCode: http.StatusOK,
		NotHeaders: []string{
			"traversed-waypoint",
		},
		Body: gstruct.Ignore(),
	}
)

func (s *testingSuite) TestServiceEntryHostnameHTTPRoute() {
	s.setNamespaceWaypointOrFail(testNamespace)
	s.applyOrFail("httproute-hostname.yaml", testNamespace)

	// svc-a has the parent ref, so only have the route there
	s.assertCurlHost(fromCurl, "se-a.serviceentry.com", hasHTTPRoute)
	s.assertCurlHost(fromCurl, "se-b.serviceentry.com", noHTTPRoute)
}

func (s *testingSuite) TestServiceEntryObjectHTTPRoute() {
	s.setNamespaceWaypointOrFail(testNamespace)
	s.applyOrFail("httproute-serviceentry.yaml", testNamespace)

	// svc-a has the parent ref, so only have the route there
	s.assertCurlHost(fromCurl, "se-a.serviceentry.com", hasHTTPRoute)
	s.assertCurlHost(fromCurl, "se-b.serviceentry.com", noHTTPRoute)
}

func (s *testingSuite) TestServiceHTTPRoute() {
	s.setNamespaceWaypointOrFail(testNamespace)
	s.applyOrFail("httproute-svc.yaml", testNamespace)

	// svc-a has the parent ref, so only have the route there
	s.assertCurlService(fromCurl, "svc-a", testNamespace, hasHTTPRoute)
	s.assertCurlService(fromCurl, "svc-b", testNamespace, noHTTPRoute)
}

func (s *testingSuite) TestGatewayHTTPRoute() {
	s.setNamespaceWaypointOrFail(testNamespace)
	s.applyOrFail("httproute-gw.yaml", testNamespace)

	// both get the route since we parent to the Gateway
	s.assertCurlService(fromCurl, "svc-a", testNamespace, hasHTTPRoute)
	s.assertCurlService(fromCurl, "svc-b", testNamespace, hasHTTPRoute)
}

func (s *testingSuite) TestHTTPRouteWeightedBackends() {
	s.Run("weighted balancing with port names", func() {
		// Test 1: Use existing svc-a and svc-b (with port names) + balance service with name
		s.applyOrFail("weighted/balance-service-with-name.yaml", testNamespace)
		s.applyOrFail("weighted/balance-httproute-existing-services.yaml", testNamespace)

		s.useWaypointLabelForTest("svc", "balance", testNamespace)
		s.runWeightedTest("balance")
	})
}

func (s *testingSuite) TestHTTPRouteWeightedBackendsNoName() {
	s.Run("weighted balancing without port names", func() {
		// Test 2: Services without port names + balance service without name
		s.applyOrFail("weighted/service-a-noname.yaml", testNamespace)
		s.applyOrFail("weighted/service-b-noname.yaml", testNamespace)
		s.applyOrFail("weighted/balance-service-noname.yaml", testNamespace)
		s.applyOrFail("weighted/balance-httproute-noname-services.yaml", testNamespace)

		s.useWaypointLabelForTest("svc", "balance-noname", testNamespace)
		s.runWeightedTest("balance-noname")
	})
}
