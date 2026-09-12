//go:build e2e

package xds_warming

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
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
)

var _ e2e.NewSuiteFunc = NewTestingSuite

var (
	setupManifest               = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	routeNewManifest            = filepath.Join(fsutils.MustGetThisDir(), "testdata", "route-new.yaml")
	routeWeightedManifest       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "route-weighted.yaml")
	backendNewManifest          = filepath.Join(fsutils.MustGetThisDir(), "testdata", "backend-new.yaml")
	startupServiceRouteManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "startup-service-route.yaml")
	startupBackendManifest      = filepath.Join(fsutils.MustGetThisDir(), "testdata", "startup-backend.yaml")
	emptyRouteManifest          = filepath.Join(fsutils.MustGetThisDir(), "testdata", "empty-route.yaml")
	canaryRouteManifest         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "canary-route.yaml")
	postrestartRouteManifest    = filepath.Join(fsutils.MustGetThisDir(), "testdata", "postrestart-route.yaml")
	metricsServiceManifest      = filepath.Join(fsutils.MustGetThisDir(), "testdata", "metrics-service.yaml")

	setup = base.TestCase{
		Manifests: []string{
			testdefaults.CurlPodManifest,
			metricsServiceManifest,
		},
	}

	testCases = map[string]*base.TestCase{
		"TestRouteUpdateToEmptyBackendPublishesTruth":          {},
		"TestWeightedRouteToEmptyBackendServesMixedTruth":      {},
		"TestInitialRouteToEmptyBackendServes503UntilReady":    {},
		"TestSteadyStateEmptyBackendSurvivesControllerRestart": {},
	}
)

const (
	gatewayNamespace   = "kgateway-base"
	gatewayName        = "gateway"
	routeName          = "xds-warming"
	hostName           = "xds-warming.example.com"
	oldBody            = "xds-warming-old"
	newBody            = "xds-warming-new"
	oldClusterName     = "kube_kgateway-base_warming-old_8080"
	newClusterName     = "kube_kgateway-base_warming-new_8080"
	startupRouteName   = "xds-warming-startup"
	startupHostName    = "xds-warming-startup.example.com"
	startupBody        = "xds-warming-startup"
	startupClusterName = "kube_kgateway-base_warming-startup_8080"
)

var proxyObjectMeta = metav1.ObjectMeta{
	Name:      gatewayName,
	Namespace: gatewayNamespace,
}

type testingSuite struct {
	*base.BaseTestingSuite
	activeCase *base.TestCase
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	cases := make(map[string]*base.TestCase, len(testCases))
	for name := range testCases {
		cases[name] = &base.TestCase{Manifests: []string{setupManifest}}
	}
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, cases),
	}
}

// Each test owns its baseline and any resources added during the scenario.
// BaseTestingSuite deletes the registered manifests without imposing a resource
// deletion order, including after an assertion fails.
func (s *testingSuite) BeforeTest(suiteName, testName string) {
	s.activeCase = &base.TestCase{Manifests: []string{setupManifest}}
	s.TestCases[testName] = s.activeCase
	s.BaseTestingSuite.BeforeTest(suiteName, testName)
}

func (s *testingSuite) AfterTest(suiteName, testName string) {
	defer s.BaseTestingSuite.AfterTest(suiteName, testName)
	s.assertNoXdsInvariantViolations()
}

func (s *testingSuite) applyManifest(manifest string) {
	// Route retargeting updates the route already owned by setupManifest.
	if manifest != routeNewManifest && manifest != routeWeightedManifest {
		s.activeCase.Manifests = append(s.activeCase.Manifests, manifest)
	}
	s.ApplyManifests(&base.TestCase{Manifests: []string{manifest}})
}

// TestRouteUpdateToEmptyBackendPublishesTruth pins stock-parity presence
// semantics (#14352): retargeting a route to a Service whose EndpointSlice
// exists but is empty publishes the flip as the backend's truth — the route
// answers 503 until endpoints arrive, and it must NOT pin route/listener/
// secret updates behind the empty backend, because "empty forever, on
// purpose" (scale-to-zero, ExternalName shapes) is a production-proven
// steady state.
func (s *testingSuite) TestRouteUpdateToEmptyBackendPublishesTruth() {
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(
		s.Ctx,
		routeName,
		gatewayNamespace,
		gwv1.RouteConditionAccepted,
		metav1.ConditionTrue,
	)
	s.assertGatewayEventuallyServes(hostName, oldBody)
	s.assertActiveClusterIsPresent(oldClusterName)

	s.applyManifest(routeNewManifest)
	s.eventuallyRouteObserved(routeName)

	// The flip publishes: the new cluster reaches the served CDS and the
	// route answers 503 (no healthy upstream) — the truthful state of an
	// endpoint-less backend, not an indefinite hold.
	s.assertActiveClusterIsPresent(newClusterName)
	s.assertGatewayEventuallyStatus(hostName, http.StatusServiceUnavailable, time.Minute, 500*time.Millisecond)

	s.applyManifest(backendNewManifest)
	s.TestInstallation.AssertionsT(s.T()).EventuallyObjectsExist(
		s.Ctx,
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "warming-new", Namespace: gatewayNamespace}},
	)
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(
		s.Ctx,
		gatewayNamespace,
		metav1.ListOptions{LabelSelector: testdefaults.WellKnownAppLabel + "=warming-new"},
		time.Minute,
		500*time.Millisecond,
	)

	s.assertGatewayEventuallyServes(hostName, newBody)
	s.assertGatewayServesConsistently(hostName, newBody, 5*time.Second, time.Second)
}

// TestWeightedRouteToEmptyBackendServesMixedTruth pins stock-parity presence
// semantics for a weighted split where one target's EndpointSlice exists but
// is empty: the split publishes immediately, so requests weighted to the
// empty target answer 503 while the rest keep serving the old backend —
// accurate per-target degradation instead of an indefinite hold (#14352).
func (s *testingSuite) TestWeightedRouteToEmptyBackendServesMixedTruth() {
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(
		s.Ctx,
		routeName,
		gatewayNamespace,
		gwv1.RouteConditionAccepted,
		metav1.ConditionTrue,
	)
	s.assertGatewayEventuallyServes(hostName, oldBody)
	s.assertActiveClusterIsPresent(oldClusterName)

	s.applyManifest(routeWeightedManifest)
	s.eventuallyRouteObserved(routeName)

	// The split publishes: the empty target's cluster reaches the served CDS
	// and its share of requests answers 503 while the old backend's share
	// keeps serving.
	s.assertActiveClusterIsPresent(newClusterName)
	s.assertGatewayEventuallyMixes(hostName, oldBody, http.StatusServiceUnavailable, time.Minute, time.Second)

	s.applyManifest(backendNewManifest)
	s.TestInstallation.AssertionsT(s.T()).EventuallyObjectsExist(
		s.Ctx,
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "warming-new", Namespace: gatewayNamespace}},
	)
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(
		s.Ctx,
		gatewayNamespace,
		metav1.ListOptions{LabelSelector: testdefaults.WellKnownAppLabel + "=warming-new"},
		time.Minute,
		500*time.Millisecond,
	)

	s.assertActiveClusterIsPresent(newClusterName)
	s.assertGatewayEventuallyServesAll(hostName, []string{oldBody, newBody}, 30*time.Second, time.Second)
}

// TestInitialRouteToEmptyBackendServes503UntilReady pins stock-parity
// presence semantics for a brand-new route whose Service exists with no
// endpoints: the route publishes and answers 503 (the backend's truth) until
// endpoints arrive — it does not stay invisible behind a hold (#14352).
func (s *testingSuite) TestInitialRouteToEmptyBackendServes503UntilReady() {
	s.applyManifest(startupServiceRouteManifest)
	s.eventuallyRouteObserved(startupRouteName)

	s.assertActiveClusterIsPresent(startupClusterName)
	s.assertGatewayEventuallyStatus(startupHostName, http.StatusServiceUnavailable, time.Minute, 500*time.Millisecond)

	s.applyManifest(startupBackendManifest)
	s.TestInstallation.AssertionsT(s.T()).EventuallyObjectsExist(
		s.Ctx,
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "warming-startup", Namespace: gatewayNamespace}},
	)
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(
		s.Ctx,
		gatewayNamespace,
		metav1.ListOptions{LabelSelector: testdefaults.WellKnownAppLabel + "=warming-startup"},
		time.Minute,
		500*time.Millisecond,
	)

	s.assertActiveClusterIsPresent(startupClusterName)
	s.assertGatewayEventuallyServes(startupHostName, startupBody)
	s.assertGatewayServesConsistently(startupHostName, startupBody, 5*time.Second, time.Second)
}

// TestSteadyStateEmptyBackendSurvivesControllerRestart pins the #14352
// failure shape end to end: a shared gateway referencing a backend that
// legitimately never has endpoints (scale-to-zero / ExternalName shape —
// here, a Service with no Deployment). The emptiness is a steady state, not
// a transient race, and the publication path must treat it as truth
// everywhere — in particular it must never enter a withhold that only
// converges if the backend gains endpoints:
//
//  1. a new route to the empty backend publishes and answers 503 — it never
//     pins route/listener/secret updates behind the empty backend;
//  2. unrelated route updates keep flowing while the empty reference exists;
//  3. after a CONTROLLER restart (empty snapshot cache, every client warm
//     and re-gated at once), proxies resume receiving updates within the
//     publish budget;
//  4. a gateway rollout after the restart brings up a fresh proxy pod that
//     goes Ready instead of starving at "cm init: initializing cds" — the
//     crash-loop symptom reported in #14352.
func (s *testingSuite) TestSteadyStateEmptyBackendSurvivesControllerRestart() {
	s.assertGatewayEventuallyServes(hostName, oldBody)

	// A route to the permanently-empty backend publishes as truth: 503 (no
	// healthy upstream), not an indefinite hold.
	s.applyManifest(emptyRouteManifest)
	s.eventuallyRouteObserved("xds-warming-empty")
	s.assertGatewayEventuallyStatus("xds-warming-empty.example.com", http.StatusServiceUnavailable, time.Minute, time.Second)

	// Unrelated route updates must keep flowing despite the steady-state
	// empty reference.
	s.applyManifest(canaryRouteManifest)
	s.assertGatewayEventuallyServes("xds-warming-canary.example.com", oldBody)

	// Controller restart: the snapshot cache starts empty and every rebuild
	// carries the steady-state empty gap, so warm clients depend on the
	// budget-bounded truth publish to ever receive config again.
	err := s.TestInstallation.Actions.Kubectl().RestartDeploymentAndWait(s.Ctx, "kgateway",
		"-n", s.TestInstallation.Metadata.InstallNamespace)
	s.Require().NoError(err, "can restart the kgateway controller")

	// Existing traffic is never interrupted (Envoy keeps its last config).
	s.assertGatewayEventuallyServes(hostName, oldBody)

	// The hole-1 regression check: a route change applied AFTER the restart
	// must become effective. Pre-fix, the warm client was withheld forever
	// and this route never appeared.
	s.applyManifest(postrestartRouteManifest)
	s.eventuallyRouteObserved("xds-warming-postrestart")
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		statusCode, body, err := s.gatewayResponse("xds-warming-postrestart.example.com")
		g.Expect(err).NotTo(gomega.HaveOccurred(), "gateway request should complete")
		g.Expect(statusCode).To(gomega.Equal(http.StatusOK), "gateway returned body: %s", body)
		g.Expect(body).To(gomega.ContainSubstring(oldBody))
	}).WithContext(s.Ctx).WithTimeout(2*time.Minute).WithPolling(time.Second).Should(gomega.Succeed(),
		"a route created after a controller restart must become effective despite a steady-state empty reference")

	// The #14352 crashloop check: a fresh proxy pod (gateway rollout) must
	// receive a snapshot and go Ready. Pre-fix it starved at cds init and
	// the rollout never completed.
	err = s.TestInstallation.Actions.Kubectl().RestartDeploymentAndWait(s.Ctx, gatewayName,
		"-n", gatewayNamespace)
	s.Require().NoError(err, "a fresh gateway proxy pod must go Ready despite a steady-state empty reference")
	s.assertGatewayEventuallyServes(hostName, oldBody)
}

// assertNoXdsInvariantViolations scrapes the controller's metrics endpoint
// and fails if either xDS invariant counter recorded a violation:
//
//   - kgateway_xds_snapshot_perclient_inconsistent_snapshots_total: a
//     published snapshot failed Snapshot.Consistent() (recorded because the
//     install runs with KGW_XDS_SNAPSHOT_CONSISTENCY_CHECK);
//   - kgateway_xds_nacks_total: a client rejected a published response and is
//     serving older config than the control plane believes.
//
// Both counters only materialize on the first violation, so absence passes.
func (s *testingSuite) assertNoXdsInvariantViolations() {
	metricsService := metav1.ObjectMeta{
		Name:      "kgateway-metrics",
		Namespace: s.TestInstallation.Metadata.InstallNamespace,
	}
	curlResponse, err := s.TestInstallation.ClusterContext.Cli.CurlFromPod(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		curl.WithHost(kubeutils.ServiceFQDN(metricsService)),
		curl.WithPort(9092),
		curl.WithPath("/metrics"),
		curl.WithConnectionTimeout(5),
	)
	s.Require().NoError(err, "can scrape controller metrics")

	response := transforms.WithCurlResponse(curlResponse)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	s.Require().NoError(err, "can read controller metrics body")

	violations := map[string]string{
		"kgateway_xds_snapshot_perclient_inconsistent_snapshots_total": "the controller published a snapshot that failed Snapshot.Consistent()",
		"kgateway_xds_nacks_total":                                     "a client NACKed a published xDS response and is serving older config",
	}
	for line := range strings.SplitSeq(string(body), "\n") {
		for metric, description := range violations {
			if !strings.HasPrefix(line, metric) {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 2 || fields[len(fields)-1] == "0" {
				continue
			}
			s.Require().Failf("xds invariant violated",
				"%s; this is a kgateway bug: %s", description, line)
		}
	}
}

func (s *testingSuite) assertGatewayEventuallyStatus(host string, status int, timeout, poll time.Duration) {
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		statusCode, body, err := s.gatewayResponse(host)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "gateway request should complete")
		g.Expect(statusCode).To(gomega.Equal(status), "gateway returned body: %s", body)
	}).WithContext(s.Ctx).WithTimeout(timeout).WithPolling(poll).Should(gomega.Succeed())
}

// assertGatewayEventuallyMixes asserts that, within a burst of requests, the
// gateway serves bodySubstring on some and answers status on others — the
// signature of a weighted split where one target is legitimately empty.
func (s *testingSuite) assertGatewayEventuallyMixes(host, bodySubstring string, status int, timeout, poll time.Duration) {
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		var sawBody, sawStatus bool
		observed := make([]string, 0, 12)
		for range 12 {
			statusCode, body, err := s.gatewayResponse(host)
			g.Expect(err).NotTo(gomega.HaveOccurred(), "gateway request should complete")
			observed = append(observed, fmt.Sprintf("%d:%s", statusCode, body))
			if statusCode == http.StatusOK && strings.Contains(body, bodySubstring) {
				sawBody = true
			}
			if statusCode == status {
				sawStatus = true
			}
			if sawBody && sawStatus {
				return
			}
		}
		g.Expect(sawBody && sawStatus).To(gomega.BeTrue(), "observed responses: %v", observed)
	}).WithContext(s.Ctx).WithTimeout(timeout).WithPolling(poll).Should(gomega.Succeed())
}

func (s *testingSuite) eventuallyRouteObserved(routeName string) {
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		route := &gwv1.HTTPRoute{}
		err := s.TestInstallation.ClusterContext.Client.Get(
			s.Ctx,
			types.NamespacedName{Name: routeName, Namespace: gatewayNamespace},
			route,
		)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "can get HTTPRoute %s/%s", gatewayNamespace, routeName)

		generation := route.GetGeneration()
		g.Expect(route.Status.Parents).NotTo(gomega.BeEmpty(), "HTTPRoute should have parent status")
		for _, parent := range route.Status.Parents {
			accepted := apimeta.FindStatusCondition(parent.Conditions, string(gwv1.RouteConditionAccepted))
			if accepted != nil && accepted.Status == metav1.ConditionTrue && accepted.ObservedGeneration >= generation {
				return
			}
		}
		g.Expect(false).To(gomega.BeTrue(), "HTTPRoute %s/%s has not observed generation %d; status: %+v",
			gatewayNamespace, routeName, generation, route.Status)
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
