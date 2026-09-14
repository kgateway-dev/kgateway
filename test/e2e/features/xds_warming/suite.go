//go:build e2e

// Package xds_warming pins what happens to LIVE traffic while a route-referenced
// backend has no endpoints: the control plane publishes the route immediately, so
// the data plane fails those requests (503, "no healthy upstream") until the
// endpoints arrive, rather than holding the old route in place.
//
// This is the complement of xds_starvation, which pins that a never-ready
// reference does not withhold configuration or stop proxies becoming Ready. That
// suite covers isolation and startup; this one covers the live retarget and the
// weighted split.
//
// Do not relax these into make-before-break assertions. The readiness gate that
// held a route flip until its endpoints were ready (#13868) was reverted in
// #14380 precisely because it could stay unsatisfied forever.
package xds_warming

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	"github.com/kgateway-dev/kgateway/v2/test/envoyutils/admincli"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/transforms"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

var (
	setupManifest         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	routeNewManifest      = filepath.Join(fsutils.MustGetThisDir(), "testdata", "route-new.yaml")
	routeWeightedManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "route-weighted.yaml")
	backendNewManifest    = filepath.Join(fsutils.MustGetThisDir(), "testdata", "backend-new.yaml")

	setup = base.TestCase{
		Manifests: []string{
			testdefaults.CurlPodManifest,
			setupManifest,
		},
	}

	testCases = map[string]*base.TestCase{
		"TestRetargetToEndpointlessBackendServes503UntilItHasEndpoints": {},
		"TestWeightedSplitWithEndpointlessBackendFailsOnlyItsShare":     {},
	}
)

const (
	gatewayNamespace = "kgateway-base"
	gatewayName      = "gateway"
	routeName        = "xds-warming"
	hostName         = "xds-warming.example.com"
	oldBody          = "xds-warming-old"
	newBody          = "xds-warming-new"
	oldClusterName   = "kube_kgateway-base_warming-old_8080"
	newClusterName   = "kube_kgateway-base_warming-new_8080"
)

var proxyObjectMeta = metav1.ObjectMeta{
	Name:      gatewayName,
	Namespace: gatewayNamespace,
}

type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

func (s *testingSuite) TearDownSuite() {
	if testutils.ShouldSkipCleanup(s.T()) {
		return
	}
	s.BaseTestingSuite.TearDownSuite()
}

func (s *testingSuite) AfterTest(suiteName, testName string) {
	s.BaseTestingSuite.AfterTest(suiteName, testName)

	if testutils.ShouldSkipCleanup(s.T()) {
		return
	}

	for _, manifest := range []string{
		backendNewManifest,
	} {
		err := s.TestInstallation.Actions.Kubectl().DeleteFileSafe(s.Ctx, manifest)
		s.Require().NoError(err, "can clean up %s", manifest)
	}

	err := s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, setupManifest)
	s.Require().NoError(err, "can restore baseline warming route")
}

// TestRetargetToEndpointlessBackendServes503UntilItHasEndpoints pins what a live
// route retarget onto a backend that has no endpoints actually does.
//
// The control plane publishes the retarget immediately: it does not withhold the
// route until the new cluster has endpoints. Envoy therefore applies the new
// route straight away, and because the destination cluster exists (kgateway emits
// a cluster for every backend) but has no endpoints, requests fail with 503 "no
// healthy upstream" rather than being routed to the previous backend.
//
// This is deliberately NOT make-before-break, and the assertion below must not be
// relaxed into one: the readiness gate that used to hold the flip (#13868) was
// reverted in #14380 because it could stay unsatisfied forever, stranding warm
// proxies on stale endpoints and starving new pods. The unit-level counterpart is
// TestSnapshotPerClientPublishesWithMissingOrUnusableEndpoints.
func (s *testingSuite) TestRetargetToEndpointlessBackendServes503UntilItHasEndpoints() {
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(
		s.Ctx,
		routeName,
		gatewayNamespace,
		gwv1.RouteConditionAccepted,
		metav1.ConditionTrue,
	)
	s.assertGatewayEventuallyServes(hostName, oldBody)
	s.assertActiveClusterIsPresent(oldClusterName)

	err := s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, routeNewManifest)
	s.Require().NoError(err, "can retarget route to new service before endpoints exist")
	s.eventuallyRouteObserved(routeName, gatewayNamespace)

	// The cluster is published even though it has no endpoints, which is why the
	// failure is "no healthy upstream" and not "no cluster".
	s.assertActiveClusterIsPresent(newClusterName)
	s.assertGatewayStatusEventually(hostName, http.StatusServiceUnavailable)
	s.assertGatewayStatusConsistently(hostName, http.StatusServiceUnavailable, 10*time.Second, 500*time.Millisecond)

	err = s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, backendNewManifest)
	s.Require().NoError(err, "can create delayed new backend deployment")
	s.eventuallyBackendPodsRunning("warming-new")

	s.assertGatewayEventuallyServes(hostName, newBody)
	s.assertGatewayServesConsistently(hostName, newBody, 5*time.Second, time.Second)
}

// TestWeightedSplitWithEndpointlessBackendFailsOnlyItsShare pins that an
// endpoint-less backend in a weighted split degrades only its own share of the
// traffic. Requests balanced onto the endpoint-less cluster fail; requests
// balanced onto the healthy one keep succeeding. Once the missing backend has
// endpoints, both destinations serve.
func (s *testingSuite) TestWeightedSplitWithEndpointlessBackendFailsOnlyItsShare() {
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(
		s.Ctx,
		routeName,
		gatewayNamespace,
		gwv1.RouteConditionAccepted,
		metav1.ConditionTrue,
	)
	s.assertGatewayEventuallyServes(hostName, oldBody)
	s.assertActiveClusterIsPresent(oldClusterName)

	err := s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, routeWeightedManifest)
	s.Require().NoError(err, "can update route to weighted old/new backends before new endpoints exist")
	s.eventuallyRouteObserved(routeName, gatewayNamespace)
	s.assertActiveClusterIsPresent(newClusterName)

	// Both outcomes must appear: the healthy half still serves, the other half fails.
	s.assertGatewayEventuallyServesMixed(hostName, oldBody, http.StatusServiceUnavailable)

	err = s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, backendNewManifest)
	s.Require().NoError(err, "can create delayed new backend deployment")
	s.eventuallyBackendPodsRunning("warming-new")

	s.assertGatewayEventuallyServesAll(hostName, []string{oldBody, newBody}, 30*time.Second, time.Second)
}

func (s *testingSuite) eventuallyRouteObserved(routeName, routeNamespace string) {
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		route := &gwv1.HTTPRoute{}
		err := s.TestInstallation.ClusterContext.Client.Get(
			s.Ctx,
			types.NamespacedName{Name: routeName, Namespace: routeNamespace},
			route,
		)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "can get HTTPRoute %s/%s", routeNamespace, routeName)

		generation := route.GetGeneration()
		g.Expect(route.Status.Parents).NotTo(gomega.BeEmpty(), "HTTPRoute should have parent status")
		for _, parent := range route.Status.Parents {
			accepted := apimeta.FindStatusCondition(parent.Conditions, string(gwv1.RouteConditionAccepted))
			if accepted != nil && accepted.Status == metav1.ConditionTrue && accepted.ObservedGeneration >= generation {
				return
			}
		}
		g.Expect(false).To(gomega.BeTrue(), "HTTPRoute %s/%s has not observed generation %d; status: %+v",
			routeNamespace, routeName, generation, route.Status)
	}).WithContext(s.Ctx).WithTimeout(time.Minute).WithPolling(500 * time.Millisecond).Should(gomega.Succeed())
}

func (s *testingSuite) assertActiveClusterIsPresent(clusterName string) {
	s.TestInstallation.AssertionsT(s.T()).AssertEnvoyAdminApi(s.Ctx, proxyObjectMeta, func(ctx context.Context, adminClient *admincli.Client) {
		s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
			clusters, err := adminClient.GetDynamicClusters(ctx)
			g.Expect(err).NotTo(gomega.HaveOccurred(), "can get dynamic active clusters")
			_, ok := clusters[clusterName]
			g.Expect(ok).To(gomega.BeTrue(), "cluster %s should be active", clusterName)
		}).WithContext(ctx).WithTimeout(time.Minute).WithPolling(time.Second).Should(gomega.Succeed())
	})
}

func (s *testingSuite) assertGatewayEventuallyServes(host, bodySubstring string) {
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		s.gatewayCurlOptions(host),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring(bodySubstring),
		},
	)
}

func (s *testingSuite) assertGatewayServesConsistently(host, bodySubstring string, window, poll time.Duration) {
	s.assertGatewayServesOnce(host, bodySubstring)

	s.TestInstallation.AssertionsT(s.T()).Gomega.Consistently(func(g gomega.Gomega) {
		statusCode, body, err := s.gatewayResponse(host)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "gateway request should complete")
		g.Expect(statusCode).To(gomega.Equal(http.StatusOK), "gateway returned body: %s", body)
		g.Expect(body).To(gomega.ContainSubstring(bodySubstring))
	}).WithContext(s.Ctx).WithTimeout(window).WithPolling(poll).Should(gomega.Succeed())
}

func (s *testingSuite) assertGatewayServesOnce(host, bodySubstring string) {
	statusCode, body, err := s.gatewayResponse(host)
	s.Require().NoError(err)
	s.Require().Equal(http.StatusOK, statusCode, "gateway returned body: %s", body)
	s.Require().Contains(body, bodySubstring)
}

func (s *testingSuite) assertGatewayEventuallyServesAll(host string, bodySubstrings []string, timeout, poll time.Duration) {
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		seen := make(map[string]bool, len(bodySubstrings))
		observedBodies := make([]string, 0, 12)

		for range 12 {
			statusCode, body, err := s.gatewayResponse(host)
			g.Expect(err).NotTo(gomega.HaveOccurred(), "gateway request should complete")
			g.Expect(statusCode).To(gomega.Equal(http.StatusOK), "gateway returned body: %s", body)

			observedBodies = append(observedBodies, body)
			for _, bodySubstring := range bodySubstrings {
				if strings.Contains(body, bodySubstring) {
					seen[bodySubstring] = true
				}
			}
			if len(seen) == len(bodySubstrings) {
				return
			}
		}

		missing := make([]string, 0, len(bodySubstrings))
		for _, bodySubstring := range bodySubstrings {
			if !seen[bodySubstring] {
				missing = append(missing, bodySubstring)
			}
		}
		g.Expect(missing).To(gomega.BeEmpty(), "observed bodies: %v", observedBodies)
	}).WithContext(s.Ctx).WithTimeout(timeout).WithPolling(poll).Should(gomega.Succeed())
}

func (s *testingSuite) assertGatewayStatusConsistently(host string, status int, window, poll time.Duration) {
	s.assertGatewayStatusOnce(host, status)

	s.TestInstallation.AssertionsT(s.T()).Gomega.Consistently(func(g gomega.Gomega) {
		statusCode, body, err := s.gatewayResponse(host)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "gateway request should complete")
		g.Expect(statusCode).To(gomega.Equal(status), "gateway returned body: %s", body)
	}).WithContext(s.Ctx).WithTimeout(window).WithPolling(poll).Should(gomega.Succeed())
}

func (s *testingSuite) assertGatewayStatusOnce(host string, status int) {
	statusCode, body, err := s.gatewayResponse(host)
	s.Require().NoError(err)
	s.Require().Equal(status, statusCode, "gateway returned body: %s", body)
}

func (s *testingSuite) gatewayResponse(host string) (int, string, error) {
	curlResponse, err := s.TestInstallation.ClusterContext.Cli.CurlFromPod(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		s.gatewayCurlOptions(host)...,
	)
	if err != nil {
		return 0, "", err
	}

	response := transforms.WithCurlResponse(curlResponse)
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return 0, "", fmt.Errorf("read gateway response body: %w", err)
	}
	return response.StatusCode, string(body), nil
}

func (s *testingSuite) gatewayCurlOptions(host string) []curl.Option {
	return []curl.Option{
		curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
		curl.WithHostHeader(host),
		curl.WithPort(80),
		curl.WithPath("/"),
		curl.WithConnectionTimeout(2),
	}
}

func (s *testingSuite) assertGatewayStatusEventually(host string, status int) {
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		statusCode, body, err := s.gatewayResponse(host)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "gateway request should complete")
		g.Expect(statusCode).To(gomega.Equal(status), "gateway returned body: %s", body)
	}).WithContext(s.Ctx).WithTimeout(time.Minute).WithPolling(500 * time.Millisecond).Should(gomega.Succeed())
}

// assertGatewayEventuallyServesMixed requires both outcomes to appear across a
// burst of requests: a success carrying bodySubstring, and a failure with
// failStatus. It is how a weighted split with one endpoint-less destination
// presents, and it fails if traffic is either entirely healthy or entirely
// failing.
func (s *testingSuite) assertGatewayEventuallyServesMixed(host, bodySubstring string, failStatus int) {
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		var sawSuccess, sawFailure bool
		observed := make([]string, 0, 20)

		for range 20 {
			statusCode, body, err := s.gatewayResponse(host)
			g.Expect(err).NotTo(gomega.HaveOccurred(), "gateway request should complete")
			observed = append(observed, strconv.Itoa(statusCode))

			switch {
			case statusCode == http.StatusOK && strings.Contains(body, bodySubstring):
				sawSuccess = true
			case statusCode == failStatus:
				sawFailure = true
			}
			if sawSuccess && sawFailure {
				return
			}
		}

		g.Expect(sawSuccess).To(gomega.BeTrue(), "expected some requests to reach the healthy backend; statuses: %v", observed)
		g.Expect(sawFailure).To(gomega.BeTrue(), "expected some requests to fail on the endpoint-less backend; statuses: %v", observed)
	}).WithContext(s.Ctx).WithTimeout(time.Minute).WithPolling(time.Second).Should(gomega.Succeed())
}

func (s *testingSuite) eventuallyBackendPodsRunning(app string) {
	s.TestInstallation.AssertionsT(s.T()).EventuallyObjectsExist(
		s.Ctx,
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: app, Namespace: gatewayNamespace}},
	)
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(
		s.Ctx,
		gatewayNamespace,
		metav1.ListOptions{LabelSelector: testdefaults.WellKnownAppLabel + "=" + app},
		time.Minute,
		500*time.Millisecond,
	)
}
