//go:build e2e

package udproute

import (
	"context"
	"strconv"
	"strings"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

// testingSuite exercises UDPRoute end to end. A Gateway UDP listener routes to CoreDNS
// backends, verified by exec-ing dig from an in-cluster client pod against the gateway.
type testingSuite struct {
	*base.BaseTestingSuite
	cancel context.CancelFunc
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	udpCtx, cancel := context.WithTimeout(ctx, ctxTimeout)
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(udpCtx, testInst, base.TestCase{}, testCases,
			base.WithMinGwApiVersion(base.GwApiRequireUdpRoutes),
		),
		cancel: cancel,
	}
}

// SetupSuite registers the context cancel before delegating, mirroring the TCPRoute suite. The
// base SetupSuite may skip the whole suite on an older Gateway API, and testify only defers
// TearDownSuite after SetupSuite returns, so a skip would otherwise leak the timeout context.
func (s *testingSuite) SetupSuite() {
	testutils.Cleanup(s.T(), s.cancel)
	s.BaseTestingSuite.SetupSuite()
}

var testCases = map[string]*base.TestCase{
	"TestSingleBackendUDPRoute":      {},
	"TestWeightedBackendsUDPRoute":   {},
	"TestInvalidBackendDropUDPRoute": {},
	"TestSessionAffinityUDPRoute":    {},
	"TestEndpointRemovalUDPRoute":    {},
}

// TestSingleBackendUDPRoute routes a UDPRoute to a single CoreDNS backend and asserts a DNS query
// through the gateway returns that backend's static answer.
func (s *testingSuite) TestSingleBackendUDPRoute() {
	testutils.Cleanup(s.T(), func() {
		s.deleteManifests(singleBackendManifest)
		s.TestInstallation.AssertionsT(s.T()).EventuallyObjectsNotExist(s.Ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: singleNs}})
	})

	s.applyManifests(singleNs, singleBackendManifest)

	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(s.Ctx, singleGwName, singleNs, gwv1.GatewayConditionProgrammed, metav1.ConditionTrue, timeout)
	s.TestInstallation.AssertionsT(s.T()).EventuallyUDPRouteCondition(s.Ctx, singleRouteName, singleNs, gwv1.RouteConditionAccepted, metav1.ConditionTrue, timeout)
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayListenerAttachedRoutes(s.Ctx, singleGwName, singleNs, gwv1.SectionName(singleListener), 1, timeout)

	// A DNS query through the gateway resolves to backend A's static answer.
	s.assertEventualDigAnswer(singleNs, singleGwName, singleAnswerIP)
}

// TestWeightedBackendsUDPRoute routes a UDPRoute to two CoreDNS backends with weights 80/20 (each
// returns a distinguishable answer) and asserts traffic reaches both, with the heavier backend
// receiving the majority.
func (s *testingSuite) TestWeightedBackendsUDPRoute() {
	testutils.Cleanup(s.T(), func() {
		s.deleteManifests(multiBackendManifest)
		s.TestInstallation.AssertionsT(s.T()).EventuallyObjectsNotExist(s.Ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: multiNs}})
	})

	s.applyManifests(multiNs, multiBackendManifest)

	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(s.Ctx, multiGwName, multiNs, gwv1.GatewayConditionProgrammed, metav1.ConditionTrue, timeout)
	s.TestInstallation.AssertionsT(s.T()).EventuallyUDPRouteCondition(s.Ctx, multiRouteName, multiNs, gwv1.RouteConditionAccepted, metav1.ConditionTrue, timeout)
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayListenerAttachedRoutes(s.Ctx, multiGwName, multiNs, gwv1.SectionName(multiListener), 1, timeout)

	// Wait until both backends are reachable through the gateway before counting the split, so a
	// not-yet-ready backend can't skew the distribution.
	s.assertEventualDigAnswer(multiNs, multiGwName, multiAnswerA)
	s.assertEventualDigAnswer(multiNs, multiGwName, multiAnswerB)

	// Each dig uses a fresh source port, so udp_proxy load-balances per query across the weighted
	// endpoints. Over enough queries both backends are hit and the 80%-weight backend dominates.
	// The assertion is intentionally loose (both hit + majority) to stay non-flaky.
	const queries = 40
	counts := map[string]int{}
	for range queries {
		out := strings.TrimSpace(s.dig(multiNs, multiGwName))
		switch {
		case strings.Contains(out, multiAnswerA):
			counts[multiAnswerA]++
		case strings.Contains(out, multiAnswerB):
			counts[multiAnswerB]++
		}
	}
	s.Assert().Positive(counts[multiAnswerA], "backend A (weight 80) received no traffic")
	s.Assert().Positive(counts[multiAnswerB], "backend B (weight 20) received no traffic")
	s.Assert().Greater(counts[multiAnswerA], counts[multiAnswerB],
		"backend A (weight 80) should receive more traffic than backend B (weight 20), got A=%d B=%d",
		counts[multiAnswerA], counts[multiAnswerB])
}

// TestSessionAffinityUDPRoute asserts udp_proxy picks a backend once per session rather than per
// datagram. Envoy keys a session on the downstream 4-tuple, so every query sent from the same
// client source port belongs to one session and must reach the same backend of the 80/20 route,
// while TestWeightedBackendsUDPRoute shows that fresh source ports spread across both.
func (s *testingSuite) TestSessionAffinityUDPRoute() {
	testutils.Cleanup(s.T(), func() {
		s.deleteManifests(multiBackendManifest)
		s.TestInstallation.AssertionsT(s.T()).EventuallyObjectsNotExist(s.Ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: multiNs}})
	})

	s.applyManifests(multiNs, multiBackendManifest)

	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(s.Ctx, multiGwName, multiNs, gwv1.GatewayConditionProgrammed, metav1.ConditionTrue, timeout)
	s.TestInstallation.AssertionsT(s.T()).EventuallyUDPRouteCondition(s.Ctx, multiRouteName, multiNs, gwv1.RouteConditionAccepted, metav1.ConditionTrue, timeout)
	s.assertEventualDigAnswer(multiNs, multiGwName, multiAnswerA)
	s.assertEventualDigAnswer(multiNs, multiGwName, multiAnswerB)

	// Queries go out back to back, well inside the session idle timeout, so they all reuse the
	// session the first one opened.
	for _, srcPort := range []int{40001, 40002, 40003} {
		var first string
		for i := range 10 {
			out := strings.TrimSpace(s.digFrom(multiNs, multiGwName, srcPort))
			s.Require().NotEmpty(out, "query %d from source port %d got no answer", i, srcPort)
			if first == "" {
				first = out
				continue
			}
			s.Require().Equal(first, out, "query %d from source port %d reached another backend than the session's first query", i, srcPort)
		}
	}
}

// TestEndpointRemovalUDPRoute scales backend B of the 80/20 route to zero and back. Without
// endpoints, the gateway drops B's share instead of giving it to A. Once B is back, the route
// splits traffic again.
func (s *testingSuite) TestEndpointRemovalUDPRoute() {
	testutils.Cleanup(s.T(), func() {
		s.deleteManifests(multiBackendManifest)
		s.TestInstallation.AssertionsT(s.T()).EventuallyObjectsNotExist(s.Ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: multiNs}})
	})

	s.applyManifests(multiNs, multiBackendManifest)

	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(s.Ctx, multiGwName, multiNs, gwv1.GatewayConditionProgrammed, metav1.ConditionTrue, timeout)
	s.TestInstallation.AssertionsT(s.T()).EventuallyUDPRouteCondition(s.Ctx, multiRouteName, multiNs, gwv1.RouteConditionAccepted, metav1.ConditionTrue, timeout)
	s.assertEventualDigAnswer(multiNs, multiGwName, multiAnswerA)
	s.assertEventualDigAnswer(multiNs, multiGwName, multiAnswerB)

	s.Require().NoError(s.TestInstallation.Actions.Kubectl().Scale(s.Ctx, multiNs, "deployment/"+multiDeploymentB, 0))
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsNotExist(s.Ctx, multiNs, metav1.ListOptions{LabelSelector: "app=" + multiDeploymentB}, timeout)

	// Wait until the gateway stops sending sessions to B.
	gomega.NewWithT(s.T()).Eventually(func(g gomega.Gomega) {
		g.Expect(s.countDigAnswers(multiNs, multiGwName, 10)).NotTo(gomega.HaveKey(multiAnswerB))
	}, timeout).Should(gomega.Succeed(), "backend B still answers after scaling it to zero")

	// A keeps its 80% and the gateway drops B's 20%. Loose bounds keep this non-flaky.
	const queries = 40
	counts := s.countDigAnswers(multiNs, multiGwName, queries)
	s.Assert().Zero(counts[multiAnswerB], "backend B has no endpoints but answered")
	s.Assert().Positive(counts[""], "backend B's share should be dropped, not given to backend A")
	s.Assert().Greater(counts[multiAnswerA], counts[""],
		"backend A (weight 80) should answer most queries, got A=%d dropped=%d", counts[multiAnswerA], counts[""])

	s.Require().NoError(s.TestInstallation.Actions.Kubectl().Scale(s.Ctx, multiNs, "deployment/"+multiDeploymentB, 1))
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.Ctx, multiNs, metav1.ListOptions{LabelSelector: "app=" + multiDeploymentB}, timeout)
	s.assertEventualDigAnswer(multiNs, multiGwName, multiAnswerB)

	counts = s.countDigAnswers(multiNs, multiGwName, queries)
	s.Assert().Positive(counts[multiAnswerB], "backend B (weight 20) received no traffic after it came back")
	s.Assert().Greater(counts[multiAnswerA], counts[multiAnswerB],
		"backend A (weight 80) should receive more traffic than backend B (weight 20), got A=%d B=%d",
		counts[multiAnswerA], counts[multiAnswerB])
}

// TestInvalidBackendDropUDPRoute routes a UDPRoute to one valid backend (weight 20) and one
// nonexistent backend (weight 80), and asserts that the gateway drops the invalid backend's share
// instead of redistributing it. The valid backend answers a minority of queries and the gateway
// drops most of them.
func (s *testingSuite) TestInvalidBackendDropUDPRoute() {
	testutils.Cleanup(s.T(), func() {
		s.deleteManifests(invalidBackendManifest)
		s.TestInstallation.AssertionsT(s.T()).EventuallyObjectsNotExist(s.Ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: dropNs}})
	})

	s.applyManifests(dropNs, invalidBackendManifest)

	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(s.Ctx, dropGwName, dropNs, gwv1.GatewayConditionProgrammed, metav1.ConditionTrue, timeout)
	s.TestInstallation.AssertionsT(s.T()).EventuallyUDPRouteCondition(s.Ctx, dropRouteName, dropNs, gwv1.RouteConditionAccepted, metav1.ConditionTrue, timeout)

	// Wait until the valid backend is reachable through the gateway.
	s.assertEventualDigAnswer(dropNs, dropGwName, dropValidAnswer)

	// The invalid backend has weight 80, so about 80% of queries should time out instead of
	// reaching the valid backend. Loose bounds (valid answers exist, drops are the majority) keep
	// this non-flaky.
	const queries = 40
	valid, dropped := 0, 0
	for range queries {
		if strings.Contains(strings.TrimSpace(s.dig(dropNs, dropGwName)), dropValidAnswer) {
			valid++
		} else {
			dropped++
		}
	}
	s.Assert().Positive(valid, "valid backend (weight 20) should still answer some queries")
	s.Assert().Greater(dropped, valid,
		"the invalid backend's weight-80 share should be dropped (majority), got valid=%d dropped=%d", valid, dropped)
}

// dig runs a single `dig +short` A-record query for queryName through the gateway's UDP listener
// and returns stdout, the answer IPs one per line, or empty on failure.
func (s *testingSuite) dig(ns, gwName string) string {
	return s.digFrom(ns, gwName, 0)
}

// digFrom is dig sent from a fixed client source port, or from a random one when srcPort is 0.
func (s *testingSuite) digFrom(ns, gwName string, srcPort int) string {
	gwAddr := kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: gwName, Namespace: ns})
	args := []string{
		"exec", "-n", ns, clientPodName, "--",
		"dig", "+short", "+timeout=2", "+tries=1",
	}
	if srcPort != 0 {
		args = append(args, "-b", "0.0.0.0#"+strconv.Itoa(srcPort))
	}
	args = append(args, "@"+gwAddr, "-p", strconv.Itoa(udpListenerPort), queryName, "A")
	stdout, _, err := s.TestInstallation.Actions.Kubectl().Execute(s.Ctx, args...)
	if err != nil {
		return ""
	}
	return stdout
}

// countDigAnswers sends n queries from fresh source ports and counts them by answer. A query
// without an answer counts under "".
func (s *testingSuite) countDigAnswers(ns, gwName string, n int) map[string]int {
	counts := map[string]int{}
	for range n {
		counts[strings.TrimSpace(s.dig(ns, gwName))]++
	}
	return counts
}

// assertEventualDigAnswer retries a DNS query until the answer contains wantIP.
func (s *testingSuite) assertEventualDigAnswer(ns, gwName, wantIP string) {
	gomega.NewWithT(s.T()).Eventually(func(g gomega.Gomega) {
		g.Expect(s.dig(ns, gwName)).To(gomega.ContainSubstring(wantIP))
	}, timeout).Should(gomega.Succeed(), "expected a DNS answer of %s through gateway %s/%s", wantIP, ns, gwName)
}

func (s *testingSuite) applyManifests(ns string, manifests ...string) {
	for _, manifest := range manifests {
		err := s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, manifest, "-n", ns)
		s.Require().NoError(err, "Failed to apply manifest "+manifest)
	}
}

func (s *testingSuite) deleteManifests(manifests ...string) {
	for _, manifest := range manifests {
		err := s.TestInstallation.Actions.Kubectl().DeleteFileSafe(s.Ctx, manifest)
		s.Require().NoError(err, "Failed to delete manifest "+manifest)
	}
}
