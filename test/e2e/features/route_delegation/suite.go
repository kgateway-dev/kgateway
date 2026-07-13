//go:build e2e

package route_delegation

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	. "github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

var (
	setup = base.TestCase{
		Manifests: []string{commonManifest},
	}

	testCases = map[string]*base.TestCase{
		"TestBasic": {
			Manifests: []string{basicRoutesManifest},
		},
		"TestRecursive": {
			Manifests: []string{recursiveRoutesManifest},
		},
		"TestCyclic": {
			Manifests: []string{cyclicRoutesManifest},
		},
		"TestInvalidChild": {
			Manifests: []string{invalidChildRoutesManifest},
		},
		"TestHeaderQueryMatch": {
			Manifests: []string{headerQueryMatchRoutesManifest},
		},
		"TestMultipleParents": {
			Manifests: []string{multipleParentsManifest},
		},
		"TestInvalidChildValidStandalone": {
			Manifests: []string{invalidChildValidStandaloneManifest},
		},
		"TestUnresolvedChild": {
			Manifests: []string{unresolvedChildManifest},
		},
		"TestMatcherInheritance": {
			Manifests: []string{matcherInheritanceManifest},
		},
		"TestRouteWeight": {
			Manifests: []string{routeWeightManifest},
		},
		"TestPolicyMerging": {
			Manifests: []string{policyMergingManifest},
		},
	}
)

type testingSuite struct {
	*base.BaseTestingSuite
	gateway     common.Gateway
	testGateway common.Gateway
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

// This is a multi-gateway suite. The primary gateway is common.BaseGateway
// (kgateway-base/gateway, set up by the top-level harness via SetupBaseGateway).
// The secondary http-gateway-test is created by common.yaml at SetupSuite and
// is used only by TestInvalidChildValidStandalone.
func (s *testingSuite) SetupSuite() {
	s.BaseTestingSuite.SetupSuite()

	s.gateway = common.BaseGateway

	addr := s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayAddress(s.Ctx, proxyTestMeta.Name, proxyTestMeta.Namespace)
	s.testGateway = common.Gateway{
		NamespacedName: types.NamespacedName{Name: proxyTestMeta.Name, Namespace: proxyTestMeta.Namespace},
		Address:        fmt.Sprintf("%s:%d", addr, gatewayTestPort),
	}
}

func (s *testingSuite) TestBasic() {
	// Assert traffic to team1 route
	s.gateway.Send(s.T(), &testmatchers.HttpResponse{StatusCode: http.StatusOK, Body: ContainSubstring(pathTeam1)},
		curl.WithPath(pathTeam1))

	// Assert traffic to team2 route
	s.gateway.Send(s.T(), &testmatchers.HttpResponse{StatusCode: http.StatusOK, Body: ContainSubstring(pathTeam2)},
		curl.WithPath(pathTeam2))
}

func (s *testingSuite) TestRecursive() {
	// Assert traffic to team1 route
	s.gateway.Send(s.T(), &testmatchers.HttpResponse{StatusCode: http.StatusOK, Body: ContainSubstring(pathTeam1)},
		curl.WithPath(pathTeam1))

	// Assert traffic to team2 route
	s.gateway.Send(s.T(), &testmatchers.HttpResponse{StatusCode: http.StatusOK, Body: ContainSubstring(pathTeam2)},
		curl.WithPath(pathTeam2))
}

func (s *testingSuite) TestCyclic() {
	// Assert traffic to team1 route
	s.gateway.Send(s.T(), &testmatchers.HttpResponse{StatusCode: http.StatusOK, Body: ContainSubstring(pathTeam1)},
		curl.WithPath(pathTeam1))

	// Assert traffic to team2 route fails with HTTP 500 as it is a cyclic route
	s.gateway.Send(s.T(), &testmatchers.HttpResponse{StatusCode: http.StatusInternalServerError},
		curl.WithPath(pathTeam2))

	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteStatusContainsMessage(s.Ctx, routeTeam2.Name, routeTeam2.Namespace,
		"cyclic reference detected", 10*time.Second, 1*time.Second)
}

func (s *testingSuite) TestInvalidChild() {
	// Assert traffic to team1 route
	s.gateway.Send(s.T(), &testmatchers.HttpResponse{StatusCode: http.StatusOK, Body: ContainSubstring(pathTeam1)},
		curl.WithPath(pathTeam1))

	// Assert traffic to team2 route fails with HTTP 500 as the route is invalid due to specifying a hostname on the child route
	s.gateway.Send(s.T(), &testmatchers.HttpResponse{StatusCode: http.StatusInternalServerError},
		curl.WithPath(pathTeam2))

	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteStatusContainsMessage(s.Ctx, routeTeam2.Name, routeTeam2.Namespace,
		"spec.hostnames must be unset", 10*time.Second, 1*time.Second)
}

func (s *testingSuite) TestHeaderQueryMatch() {
	// Assert traffic to team1 route with matching header and query parameters
	s.gateway.Send(s.T(),
		&testmatchers.HttpResponse{StatusCode: http.StatusOK, Body: ContainSubstring(pathTeam1)},
		curl.WithPath(pathTeam1),
		curl.WithHeader("header1", "val1"),
		curl.WithHeader("headerX", "valX"),
		curl.WithQueryParameters(map[string]string{"query1": "val1", "queryX": "valX"}),
	)

	// Assert traffic to team2 child route fails with HTTP 404 as it does not match the parent's header and query parameters
	s.gateway.Send(s.T(),
		&testmatchers.HttpResponse{StatusCode: http.StatusNotFound},
		curl.WithPath(pathTeam2),
		curl.WithHeader("headerX", "valX"),
		curl.WithQueryParameters(map[string]string{"queryX": "valX"}),
	)

	// Assert traffic to team2 parent route fails with HTTP 500 due to unresolved child route
	s.gateway.Send(s.T(),
		&testmatchers.HttpResponse{StatusCode: http.StatusInternalServerError},
		curl.WithPath(pathTeam2),
		curl.WithHeader("header2", "val2"),
		curl.WithQueryParameters(map[string]string{"query2": "val2"}),
	)
}

func (s *testingSuite) TestMultipleParents() {
	// Assert traffic to parent1.com/anything/team1
	s.gateway.Send(s.T(),
		&testmatchers.HttpResponse{StatusCode: http.StatusOK, Body: ContainSubstring(pathTeam1)},
		curl.WithPath(pathTeam1),
		curl.WithHostHeader(routeParent1Host),
	)

	// Assert traffic to parent1.com/anything/team2
	s.gateway.Send(s.T(),
		&testmatchers.HttpResponse{StatusCode: http.StatusOK, Body: ContainSubstring(pathTeam2)},
		curl.WithPath(pathTeam2),
		curl.WithHostHeader(routeParent1Host),
	)

	// Assert traffic to parent2.com/anything/team1
	s.gateway.Send(s.T(),
		&testmatchers.HttpResponse{StatusCode: http.StatusOK, Body: ContainSubstring(pathTeam1)},
		curl.WithPath(pathTeam1),
		curl.WithHostHeader(routeParent2Host),
	)

	// Assert traffic to parent2.com/anything/team2 fails as it is not selected by parent2 route
	s.gateway.Send(s.T(),
		&testmatchers.HttpResponse{StatusCode: http.StatusInternalServerError},
		curl.WithPath(pathTeam2),
		curl.WithHostHeader(routeParent2Host),
	)
}

func (s *testingSuite) TestInvalidChildValidStandalone() {
	// Assert traffic to team1 route
	s.testGateway.Send(s.T(),
		&testmatchers.HttpResponse{StatusCode: http.StatusOK, Body: ContainSubstring(pathTeam1)},
		curl.WithPath(pathTeam1),
		curl.WithHostHeader(routeParentHost),
	)

	// Assert traffic to team2 route on parent hostname fails with HTTP 500 as the route is invalid due to specifying a hostname on the child route
	s.testGateway.Send(s.T(),
		&testmatchers.HttpResponse{StatusCode: http.StatusInternalServerError},
		curl.WithPath(pathTeam2),
		curl.WithHostHeader(routeParentHost),
	)

	// Assert traffic to team2 route on standalone host succeeds
	s.testGateway.Send(s.T(),
		&testmatchers.HttpResponse{StatusCode: http.StatusOK, Body: ContainSubstring(pathTeam2)},
		curl.WithPath(pathTeam2),
		curl.WithHostHeader(routeTeam2Host),
	)

	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteStatusContainsMessage(s.Ctx, routeTeam2.Name, routeTeam2.Namespace,
		"spec.hostnames must be unset", 10*time.Second, 1*time.Second)
}

func (s *testingSuite) TestUnresolvedChild() {
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteStatusContainsReason(s.Ctx, routeRoot.Name, routeRoot.Namespace,
		string(gwv1.RouteReasonBackendNotFound), 10*time.Second, 1*time.Second)
}

func (s *testingSuite) TestMatcherInheritance() {
	// Wait for both parent routes to be accepted before sending traffic
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteStatusContainsReason(s.Ctx, routeParent1.Name, routeParent1.Namespace,
		string(gwv1.RouteReasonAccepted), 10*time.Second, 1*time.Second)
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteStatusContainsReason(s.Ctx, routeParent2.Name, routeParent2.Namespace,
		string(gwv1.RouteReasonAccepted), 10*time.Second, 1*time.Second)
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteStatusContainsReason(s.Ctx, routeTeam1.Name, routeTeam1.Namespace,
		string(gwv1.RouteReasonAccepted), 10*time.Second, 1*time.Second)

	// Assert traffic on parent1's prefix
	s.gateway.Send(s.T(),
		&testmatchers.HttpResponse{StatusCode: http.StatusOK, Body: ContainSubstring("/anything/foo/child")},
		curl.WithPath("/anything/foo/child"))

	// Assert traffic on parent2's prefix
	s.gateway.Send(s.T(),
		&testmatchers.HttpResponse{StatusCode: http.StatusOK, Body: ContainSubstring("/anything/baz/child")},
		curl.WithPath("/anything/baz/child"))
}

func (s *testingSuite) TestRouteWeight() {
	// Assert traffic to /anything path prefix is always routed to svc1
	s.gateway.Send(s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       ContainSubstring(pathTeam1),
			Headers: map[string]any{
				"origin": "svc1",
			},
		},
		curl.WithPath(pathTeam1))
	// Assert traffic to /anything/team2 is also routed to svc1 since its route has higher weight
	s.gateway.Send(s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       ContainSubstring(pathTeam2),
			Headers: map[string]any{
				"origin": "svc1",
			},
		},
		curl.WithPath(pathTeam2))
}

func (s *testingSuite) TestPolicyMerging() {
	s.T().Skip("skipping. See https://github.com/kgateway-dev/kgateway/issues/13314 for details")
	// Assert traffic to parent1.com/anything/team1 uses svc1's transformation policy
	s.gateway.Send(s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       ContainSubstring(pathTeam1),
			Headers: map[string]any{
				"origin": "svc1",
			},
		},
		curl.WithPath(pathTeam1),
		curl.WithHostHeader(routeParent1Host),
	)

	// Assert traffic to parent1.com/anything/team2 uses svc2's transformation policy
	s.gateway.Send(s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       ContainSubstring(pathTeam2),
			Headers: map[string]any{
				"origin": "svc2",
			},
		},
		curl.WithPath(pathTeam2),
		curl.WithHostHeader(routeParent1Host),
	)

	// Assert traffic to parent2.com/anything/team1 uses parent2's transformation policy
	s.gateway.Send(s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       ContainSubstring(pathTeam1),
			Headers: map[string]any{
				"origin": "parent2",
			},
		},
		curl.WithPath(pathTeam1),
		curl.WithHostHeader(routeParent2Host),
	)

	// Assert traffic to parent2.com/anything/team2 uses parent2's transformation policy
	s.gateway.Send(s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       ContainSubstring(pathTeam2),
			Headers: map[string]any{
				"origin": "parent2",
			},
		},
		curl.WithPath(pathTeam2),
		curl.WithHostHeader(routeParent2Host),
	)
}
