//go:build e2e

package waypoint

import (
	"net/http"

	"github.com/onsi/gomega/gstruct"
	"istio.io/api/label"

	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

var (
	waypointLabel = label.IoIstioUseWaypoint.Name

	hasEnvoy = matchers.HttpResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]any{
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

func (s *testingSuite) useWaypointLabelForTest(kind, name, namespace string) {
	testutils.Cleanup(s.T(), func() {
		err := s.testInstallation.ClusterContext.Cli.UnsetLabel(s.ctx, kind, name, namespace, waypointLabel)
		if err != nil {
			// this could break other tests, so fail here
			s.FailNow("failed removing label", err)
		}
	})
	err := s.testInstallation.ClusterContext.Cli.SetLabel(s.ctx, kind, name, namespace, waypointLabel, gwName)
	if err != nil {
		s.FailNow("failed applying label", err)
		return
	}
}

func (s *testingSuite) TestServiceAttached() {
	s.Run("kube Service", func() {
		s.useWaypointLabelForTest("svc", "svc-a", testNamespace)

		// only svc-a should through the waypoint
		s.assertCurlService(fromCurl, "svc-a", testNamespace, hasEnvoy)
		s.assertCurlService(fromCurl, "svc-b", testNamespace, noEnvoy)
	})
	s.Run("istio ServiceEntry", func() {
		s.useWaypointLabelForTest("serviceentry", "se-a", testNamespace)

		// only se-a should through the waypoint
		s.assertCurlHost(fromCurl, "se-a.serviceentry.com", hasEnvoy)
		s.assertCurlHost(fromCurl, "se-b.serviceentry.com", noEnvoy)
	})
}

func (s *testingSuite) TestNamespaceAttached() {
	s.setNamespaceWaypointOrFail(testNamespace)

	s.Run("kube Service", func() {
		// everything goes through the waypoint
		s.assertCurlService(fromCurl, "svc-a", testNamespace, hasEnvoy)
		s.assertCurlService(fromCurl, "svc-b", testNamespace, hasEnvoy)
	})
	s.Run("istio ServiceEntry", func() {
		// including ServiceEntry
		s.assertCurlHost(fromCurl, "se-a.serviceentry.com", hasEnvoy)
		s.assertCurlHost(fromCurl, "se-b.serviceentry.com", hasEnvoy)
	})
}
