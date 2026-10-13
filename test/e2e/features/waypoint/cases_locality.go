//go:build e2e

package waypoint

import (
	"strings"
	"time"

	networking "istio.io/api/networking/v1alpha3"
	istionetworkingv1 "istio.io/client-go/pkg/apis/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

const (
	localityNamespace = "waypoint-locality-ns"
	localityGateway   = "locality-gw"

	// localityRequests is the number of requests per sampling round. With the two
	// endpoints weighted equally, each should get about half; minLocalityShare is
	// loose enough to not flake, while an endpoint that gets no traffic at all
	// fails every round.
	localityRequests = 40
	minLocalityShare = 8
)

// TestLocalityWeightWithoutLocality checks that an endpoint without a locality
// still receives traffic when another endpoint of the same backend has one.
// Envoy only records a locality group's weight when the group has a locality,
// so a group emitted without one gets weight 0 under locality-weighted LB and
// is never picked while the other locality has healthy endpoints.
func (s *testingSuite) TestLocalityWeightWithoutLocality() {
	if s.ingressUseWaypoint {
		s.T().Skip("does not depend on ingress-use-waypoint; covered by the Waypoint suite")
	}

	s.applyOrFail("locality-weight.yaml", localityNamespace)

	assertions := s.testInstallation.AssertionsT(s.T())
	assertions.EventuallyGatewayCondition(s.ctx, localityGateway, localityNamespace,
		gwv1.GatewayConditionProgrammed, metav1.ConditionTrue, readyTimeout)
	assertions.EventuallyHTTPRouteCondition(s.ctx, "locality-route", localityNamespace,
		gwv1.RouteConditionAccepted, metav1.ConditionTrue, readyTimeout)
	for _, selector := range []string{
		"app=locality-echo-a",
		"app=locality-echo-b",
		"gateway.networking.k8s.io/gateway-name=" + localityGateway,
	} {
		assertions.EventuallyPodsRunning(s.ctx, localityNamespace,
			metav1.ListOptions{LabelSelector: selector}, readyTimeout)
	}

	s.createLocalityWorkloadEntry("with-locality", s.podIP("app=locality-echo-a"), "r1/z1")
	s.createLocalityWorkloadEntry("without-locality", s.podIP("app=locality-echo-b"), "")

	curlOpts := []curl.Option{
		curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: localityGateway, Namespace: localityNamespace})),
		curl.WithPort(testAppPort),
		curl.WithHostHeader("locality.example"),
	}

	// Sample in rounds until both endpoints get their share or the deadline
	// passes. This runs in the test goroutine rather than require.Eventually,
	// whose condition goroutine can outlive the deadline and keep curling while
	// cleanup tears the fixture down.
	deadline := time.Now().Add(2 * time.Minute)
	for {
		counts := s.sampleLocalityDistribution(curlOpts)
		s.T().Logf("locality distribution over %d requests: %v", localityRequests, counts)
		if counts["echo-a"] >= minLocalityShare && counts["echo-b"] >= minLocalityShare {
			return
		}
		if time.Now().After(deadline) {
			s.FailNow("both endpoints must receive traffic; the endpoint without a locality (echo-b) must not be starved",
				"last distribution over %d requests: %v", localityRequests, counts)
		}
		time.Sleep(time.Second)
	}
}

// sampleLocalityDistribution sends localityRequests requests through the
// locality gateway and counts which echo backend answered each one.
func (s *testingSuite) sampleLocalityDistribution(curlOpts []curl.Option) map[string]int {
	counts := map[string]int{}
	for range localityRequests {
		resp, err := s.testInstallation.ClusterContext.Cli.CurlFromPod(s.ctx, fromCurl, curlOpts...)
		switch {
		case err != nil:
			counts["error"]++
		case strings.Contains(resp.StdOut, "echo-a"):
			counts["echo-a"]++
		case strings.Contains(resp.StdOut, "echo-b"):
			counts["echo-b"]++
		default:
			counts["other"]++
		}
	}
	return counts
}

// podIP returns the IP of the single running pod matching selector in the
// locality namespace.
func (s *testingSuite) podIP(selector string) string {
	pods, err := s.testInstallation.ClusterContext.Clientset.CoreV1().Pods(localityNamespace).List(
		s.ctx, metav1.ListOptions{LabelSelector: selector})
	s.Require().NoError(err, "list pods %s", selector)
	s.Require().Len(pods.Items, 1, "want exactly one pod for %s", selector)
	ip := pods.Items[0].Status.PodIP
	s.Require().NotEmpty(ip, "pod for %s has no IP", selector)
	return ip
}

// createLocalityWorkloadEntry creates a WorkloadEntry selected by the locality
// ServiceEntry. The address is a pod IP only known at runtime, so it cannot live
// in the static fixture. An empty locality leaves spec.locality unset.
func (s *testingSuite) createLocalityWorkloadEntry(name, address, locality string) {
	labels := map[string]string{"locality-test": "lb"}
	we := &istionetworkingv1.WorkloadEntry{
		Name:      name,
		Namespace: localityNamespace,
		Labels:    labels,
		Spec: networking.WorkloadEntry{
			Address:  address,
			Labels:   labels,
			Locality: locality,
			Ports:    map[string]uint32{"http": 5678},
		},
	}
	s.Require().NoError(s.testInstallation.ClusterContext.Client.Create(s.ctx, we), "create WorkloadEntry %s", name)
	testutils.Cleanup(s.T(), func() {
		s.NoError(s.testInstallation.ClusterContext.Client.Delete(s.ctx, we), "delete WorkloadEntry %s", name)
	})
}
