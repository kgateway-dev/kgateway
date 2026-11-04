//go:build e2e

package waypoint

import (
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

var ingressWaypointLabel = "istio.io/ingress-use-waypoint"

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
	}), "example.com", noHTTPRoute, "GET")
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
	}), "example.com", expected, "GET")
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
	}), "example.com", expected, "GET")
}

// TestIngressHTTPRouteServiceEntryAutoAllocatedVIP checks that an
// ingress-use-waypoint route to a ServiceEntry whose VIP is auto-allocated into
// status.addresses reaches the backend once the VIP is assigned.
func (s *testingSuite) TestIngressHTTPRouteServiceEntryAutoAllocatedVIP() {
	if !s.ingressUseWaypoint {
		s.T().Skip("only relevant when ingress-use-waypoint is enabled")
	}

	s.setNamespaceWaypointOrFail(testNamespace)
	s.applyOrFail("httproute-ingress-serviceentry.yaml", testNamespace)

	vip := s.waitForServiceEntryVIP("se-autoalloc", testNamespace)
	s.T().Logf("ServiceEntry se-autoalloc got auto-allocated VIP %q", vip)

	// Positive control: the mesh path through the waypoint is healthy.
	s.assertCurlInner(fromCurl, "se-autoalloc.serviceentry.com", "", hasHTTPRoute, "")

	// Ingress to the ServiceEntry Hostname backend must reach the backend via the waypoint.
	s.assertCurlInner(fromCurl, kubeutils.ServiceFQDN(metav1.ObjectMeta{
		Name:      "gw",
		Namespace: testNamespace,
	}), "example.com", hasHTTPRoute, "")
}

// waitForServiceEntryVIP waits for Istio to auto-allocate a VIP into the
// ServiceEntry's status.addresses and returns it.
func (s *testingSuite) waitForServiceEntryVIP(name, namespace string) string {
	var vip string
	s.Require().Eventually(func() bool {
		out, _, err := s.testInstallation.ClusterContext.Cli.Execute(s.ctx,
			"get", "serviceentry", name, "-n", namespace,
			"-o", "jsonpath={.status.addresses[0].value}")
		if err != nil {
			return false
		}
		vip = strings.TrimSpace(out)
		return vip != ""
	}, readyTimeout, 2*time.Second, "ServiceEntry %s/%s never got an auto-allocated VIP", namespace, name)
	return vip
}

func (s *testingSuite) setIngressUseWaypointLabel(kind, name, namespace string) {
	testutils.Cleanup(s.T(), func() {
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
