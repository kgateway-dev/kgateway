package waypoint

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/onsi/gomega/gstruct"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils/kubectl"
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

	// Response is forbidden
	isForbidden = matchers.HttpResponse{
		StatusCode: http.StatusForbidden,
		Body:       gstruct.Ignore(),
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

func (s *testingSuite) TestAuthzNoCurlSvcB() {
	s.setNamespaceWaypointOrFail(testNamespace)
	s.applyOrFail("authz-deny-notcurl-svc-b.yaml", testNamespace)

	// ensure waypoint attachment, and all requests fromCurl succeed
	s.assertCurlService(fromCurl, "svc-a", testNamespace, hasEnvoy)
	s.assertCurlService(fromCurl, "svc-b", testNamespace, hasEnvoy)

	// ensure authz is only applied to svc-a
	s.assertCurlService(fromNotCurl, "svc-a", testNamespace, hasEnvoy)
	s.assertCurlService(fromNotCurl, "svc-b", testNamespace, isForbidden)
}

func (s *testingSuite) TestAuthzGatewayClassRef() {
	s.setNamespaceWaypointOrFail(testNamespace)
	s.applyOrFail("authz-gatewayclass-ref.yaml", "istio-system")

	// Verify waypoint attachment
	s.assertCurlService(fromCurl, "svc-a", testNamespace, isForbidden)
	s.assertCurlService(fromCurl, "svc-b", testNamespace, isForbidden)

	// Verify that policy applies to all services for notcurl
	s.assertCurlService(fromNotCurl, "svc-a", testNamespace, isForbidden)
	s.assertCurlService(fromNotCurl, "svc-b", testNamespace, isForbidden)
}

func (s *testingSuite) TestAuthzGatewayRef() {
	s.setNamespaceWaypointOrFail(testNamespace)
	s.applyOrFail("authz-gateway-ref.yaml", testNamespace)

	// Verify waypoint attachment
	s.assertCurlService(fromCurl, "svc-a", testNamespace, isForbidden)
	s.assertCurlService(fromCurl, "svc-b", testNamespace, isForbidden)

	// Verify that policy applies to all services for notcurl
	s.assertCurlService(fromNotCurl, "svc-a", testNamespace, isForbidden)
	s.assertCurlService(fromNotCurl, "svc-b", testNamespace, isForbidden)
}

func (s *testingSuite) TestAuthzMultiService() {
	s.setNamespaceWaypointOrFail(testNamespace)
	s.applyOrFail("authz-multi-service.yaml", testNamespace)

	// Verify waypoint attachment
	s.assertCurlService(fromCurl, "svc-a", testNamespace, isForbidden)
	s.assertCurlService(fromCurl, "svc-b", testNamespace, isForbidden)

	// Verify that policy applies to all services for notcurl
	s.assertCurlService(fromNotCurl, "svc-a", testNamespace, isForbidden)
	s.assertCurlService(fromNotCurl, "svc-b", testNamespace, isForbidden)

	// repeat with POST (should be allowed)
	s.assertCurlServicePost(fromCurl, "svc-a", testNamespace, hasEnvoy)
	s.assertCurlServicePost(fromCurl, "svc-b", testNamespace, hasEnvoy)
	s.assertCurlServicePost(fromNotCurl, "svc-a", testNamespace, hasEnvoy)
	s.assertCurlServicePost(fromNotCurl, "svc-b", testNamespace, hasEnvoy)
}

func (s *testingSuite) TestAuthzComplexRule() {
	s.setNamespaceWaypointOrFail(testNamespace)
	s.applyOrFail("authz-complex-rules.yaml", testNamespace)

	type fromSpec struct {
		name string
		opts kubectl.PodExecOptions
	}
	froms := []fromSpec{
		{"curl", fromCurl},
		{"notcurl", fromNotCurl},
	}
	services := []string{"svc-a", "svc-b"}
	methods := []string{"GET", "POST"}
	paths := []string{"", "/admin/"}

	// Only these combinations are denied
	denyMap := map[string]struct{}{
		"notcurl|svc-a|GET|/admin/":  {},
		"notcurl|svc-a|POST|/admin/": {},
	}

	for _, from := range froms {
		for _, svc := range services {
			for _, method := range methods {
				for _, path := range paths {
					key := fmt.Sprintf("%s|%s|%s|%s", from.name, svc, method, path)
					expected := hasEnvoy
					if _, deny := denyMap[key]; deny {
						expected = isForbidden
					}

					s.T().Run(key, func(t *testing.T) {
						s.assertCurlGeneric(from.opts, svc, method, path, expected)
					})
				}
			}
		}
	}
}

func (s *testingSuite) TestAuthzServiceEntry() {
	s.setNamespaceWaypointOrFail(testNamespace)
	s.applyOrFail("authz-serviceentry.yaml", testNamespace)

	// ensure waypoint attachment, and all requests fromCurl succeed
	s.assertCurlHost(fromCurl, "se-a.serviceentry.com", hasEnvoy)
	s.assertCurlHost(fromCurl, "se-b.serviceentry.com", isForbidden)

	// ensure authz is only applied to svc-a
	s.assertCurlHost(fromNotCurl, "se-a.serviceentry.com", hasEnvoy)
	s.assertCurlHost(fromNotCurl, "se-b.serviceentry.com", isForbidden)

	// POST should be allowed
	s.assertCurlHostPost(fromCurl, "se-a.serviceentry.com", hasEnvoy)
	s.assertCurlHostPost(fromCurl, "se-b.serviceentry.com", hasEnvoy)
	s.assertCurlHostPost(fromNotCurl, "se-a.serviceentry.com", hasEnvoy)
	s.assertCurlHostPost(fromNotCurl, "se-b.serviceentry.com", hasEnvoy)
}
