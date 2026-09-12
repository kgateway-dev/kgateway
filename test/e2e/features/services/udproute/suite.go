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

// testingSuite exercises UDPRoute end to end: a Gateway UDP listener routing to CoreDNS
// backends, verified by exec-ing dig from an in-cluster dnsutils pod against the gateway.
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

// SetupSuite registers the context cancel before delegating, mirroring the TCPRoute suite: the
// base SetupSuite may skip the whole suite on an older Gateway API, and testify only defers
// TearDownSuite after SetupSuite returns, so a skip would otherwise leak the timeout context.
func (s *testingSuite) SetupSuite() {
	testutils.Cleanup(s.T(), s.cancel)
	s.BaseTestingSuite.SetupSuite()
}

var testCases = map[string]*base.TestCase{
	"TestSingleBackendUDPRoute":    {},
	"TestWeightedBackendsUDPRoute": {},
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
		"backend A (weight 80) should receive more traffic than backend B (weight 20); got A=%d B=%d",
		counts[multiAnswerA], counts[multiAnswerB])
}

// dig runs a single `dig +short` A-record query for queryName through the gateway's UDP listener
// and returns stdout (the answer IPs, one per line; empty on failure).
func (s *testingSuite) dig(ns, gwName string) string {
	gwAddr := kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: gwName, Namespace: ns})
	stdout, _, err := s.TestInstallation.Actions.Kubectl().Execute(s.Ctx,
		"exec", "-n", ns, clientPodName, "--",
		"dig", "+short", "+timeout=2", "+tries=1",
		"@"+gwAddr, "-p", strconv.Itoa(udpListenerPort), queryName, "A",
	)
	if err != nil {
		return ""
	}
	return stdout
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
