package waypoint

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

var (
	ingressWaypointLabel = "istio.io/ingress-use-waypoint"
)

func (s *testingSuite) TestIngressHTTPRouteWithoutLabel() {
	s.setNamespaceWaypointOrFail(testNamespace)
	s.applyOrFail("httproute-ingress.yaml", testNamespace)
	s.applyOrFail("httproute-svc.yaml", testNamespace)

	// verifying first the in-mesh traffic
	// svc-a has the parent ref, so only have the route there
	s.assertCurlService(fromCurl, "svc-a", testNamespace, hasHTTPRoute)
	s.assertCurlService(fromCurl, "svc-b", testNamespace, noHTTPRoute)

	// verifying the ingress traffic does not go through waypoint because
	// the ingress-use-waypoint label is not set
	s.assertCurlInner(fromCurl, kubeutils.ServiceFQDN(metav1.ObjectMeta{
		Name:      "gw",
		Namespace: testNamespace,
	}), "example.com", noHTTPRoute, "", "GET")
}

func (s *testingSuite) TestIngressHTTPRouteServiceLabel() {
	s.setNamespaceWaypointOrFail(testNamespace)
	s.setIngressUseWaypointLabel("svc", "svc-a", testNamespace)
	s.applyOrFail("httproute-ingress.yaml", testNamespace)
	s.applyOrFail("httproute-svc.yaml", testNamespace)

	// verifying first the in-mesh traffic
	// svc-a has the parent ref, so only have the route there
	s.assertCurlService(fromCurl, "svc-a", testNamespace, hasHTTPRoute)
	s.assertCurlService(fromCurl, "svc-b", testNamespace, noHTTPRoute)

	// verifying the ingress traffic goes through waypoint
	expected := hasHTTPRoute
	if !s.ingressUseWaypoint {
		// Verifying that if the IngressUseWaypoints is disabled in the settings,
		// the ingress traffic doesn't go through waypoint although labeled with
		// istio.io/ingress-use-waypoint=true
		expected = noHTTPRoute
	}
	s.assertCurlInner(fromCurl, kubeutils.ServiceFQDN(metav1.ObjectMeta{
		Name:      "gw",
		Namespace: testNamespace,
	}), "example.com", expected, "", "GET")
}

func (s *testingSuite) TestIngressHTTPRouteNamespaceLabel() {
	s.setNamespaceWaypointOrFail(testNamespace)
	s.setIngressUseWaypointLabel("ns", testNamespace, "")
	s.applyOrFail("httproute-ingress.yaml", testNamespace)
	s.applyOrFail("httproute-svc.yaml", testNamespace)

	// verifying first the in-mesh traffic
	// svc-a has the parent ref, so only have the route there
	s.assertCurlService(fromCurl, "svc-a", testNamespace, hasHTTPRoute)
	s.assertCurlService(fromCurl, "svc-b", testNamespace, noHTTPRoute)

	// verifying the ingress traffic goes through waypoint
	expected := hasHTTPRoute
	if !s.ingressUseWaypoint {
		// Verifying that if the IngressUseWaypoints is disabled in the settings,
		// the ingress traffic doesn't go through waypoint although labeled with
		// istio.io/ingress-use-waypoint=true
		expected = noHTTPRoute
	}
	s.assertCurlInner(fromCurl, kubeutils.ServiceFQDN(metav1.ObjectMeta{
		Name:      "gw",
		Namespace: testNamespace,
	}), "example.com", expected, "", "GET")
}

func (s *testingSuite) setIngressUseWaypointLabel(kind, name, namespace string) {
	s.T().Cleanup(func() {
		err := s.testInstallation.ClusterContext.Cli.UnsetLabel(s.ctx, kind, name, namespace, ingressWaypointLabel)
		if err != nil {
			// this could break other tests, so fail here
			s.FailNow("failed removing label", err)
		}
	})
	err := s.testInstallation.ClusterContext.Cli.SetLabel(s.ctx, kind, name, namespace, ingressWaypointLabel, "true")
	if err != nil {
		s.FailNow("failed applying label", err)
		return
	}
}
