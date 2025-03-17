package waypoint

import (
	"net/http"

	"github.com/onsi/gomega/gstruct"
	"istio.io/api/label"

	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var (
	waypointLabel = label.IoIstioUseWaypoint.Name

	hasEnvoy = matchers.HttpResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]interface{}{
			"server": "envoy",
		},
		Body: gstruct.Ignore(),
	}

	noEnvoy = matchers.HttpResponse{
		StatusCode: http.StatusOK,
		NotHeaders: []string{
			"x-envoy-upstream-service-time",
		},
		Body: gstruct.Ignore(),
	}
)

func (s *testingSuite) TestServiceAttached() {
	s.T().Cleanup(func() {
		err := s.testInstallation.ClusterContext.Cli.UnsetLabel(s.ctx, "svc", "svc-a", testNamespace, waypointLabel)
		if err != nil {
			// this could break other tests
			s.FailNow("failed removing label", err)
		}
	})
	err := s.testInstallation.ClusterContext.Cli.SetLabel(s.ctx, "svc", "svc-a", testNamespace, waypointLabel, gwName)
	if err != nil {
		s.FailNow("failed applying label", err)
		return
	}

	// only svc-a should through the waypoint
	s.assertCurlService(fromCurl, "svc-a", testNamespace, hasEnvoy)
	s.assertCurlService(fromCurl, "svc-b", testNamespace, noEnvoy)
}

func (s *testingSuite) TestNamespaceAttached() {
	s.setNamespaceWaypointOrFail(testNamespace)

	// both go through the waypoint
	s.assertCurlService(fromCurl, "svc-a", testNamespace, hasEnvoy)
	s.assertCurlService(fromCurl, "svc-b", testNamespace, hasEnvoy)
}
