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
