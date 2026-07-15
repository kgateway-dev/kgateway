//go:build e2e

// Package xds_starvation pins the anti-starvation properties of per-client
// xDS publication (#14184): a gateway whose config contains a reference that
// can never become ready (an ExternalName Service, which produces no
// EndpointSlices) must still publish config, keep its pods Ready across
// rollouts, and keep receiving updates across controller restarts. Under the
// former whole-snapshot readiness gates (#13868, reverted) this exact shape
// withheld the entire snapshot forever: warm proxies were stranded and fresh
// pods crash-looped (#14352).
package xds_starvation

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/transforms"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

var (
	setupManifest            = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	postrestartRouteManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "postrestart-route.yaml")

	setup = base.TestCase{
		Manifests: []string{
			testdefaults.CurlPodManifest,
			// The valid/postrestart routes reference the nginx Service in the
			// "nginx" namespace, so it must exist before setupManifest applies
			// the ReferenceGrant into it. (v2.3.x has no shared nginx-shared
			// backend, so this suite provisions its own.)
			testdefaults.NginxPodManifest,
			setupManifest,
		},
	}

	testCases = map[string]*base.TestCase{
		"TestServesDespiteNeverReadyReference":      {},
		"TestGatewayRolloutSurvives":                {},
		"TestControllerRestartDoesNotStrandClients": {},
	}
)

const (
	gatewayName      = "xds-starvation-gw"
	gatewayNamespace = "default"
	validHost        = "valid.example.com"
	extnameHost      = "extname.example.com"
	postrestartHost  = "postrestart.example.com"
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

func (s *testingSuite) AfterTest(suiteName, testName string) {
	s.BaseTestingSuite.AfterTest(suiteName, testName)

	if testutils.ShouldSkipCleanup(s.T()) {
		return
	}
	err := s.TestInstallation.Actions.Kubectl().DeleteFileSafe(s.Ctx, postrestartRouteManifest)
	s.Require().NoError(err, "can clean up %s", postrestartRouteManifest)
}

// TestServesDespiteNeverReadyReference is the core anti-starvation pin: with
// a route to a never-ready ExternalName backend in the config, the gateway
// still publishes, its pod becomes Ready, and the valid route serves. Under
// the reverted gates, this exact configuration withheld the whole snapshot
// and the curl below would never succeed. The never-ready route itself must
// answer (with an upstream error), not hang the rest of the config.
func (s *testingSuite) TestServesDespiteNeverReadyReference() {
	s.assertEventuallyServesNginx(validHost)

	// The never-ready route fails alone: any HTTP error status is acceptable
	// (503 once the cluster settles; the point is the response arrives and
	// nothing else was withheld).
	s.assertEventuallyStatusAtLeast(extnameHost, http.StatusInternalServerError, 2*time.Minute)
}

// TestGatewayRolloutSurvives replaces every proxy pod while the never-ready
// reference exists. Fresh pods have no cached snapshot to fall back on, so
// under the reverted gates they were starved of all config and crash-looped
// (#14352); now the rollout must complete and traffic must resume.
func (s *testingSuite) TestGatewayRolloutSurvives() {
	s.assertEventuallyServesNginx(validHost)

	err := s.TestInstallation.Actions.Kubectl().RestartDeploymentAndWait(s.Ctx, gatewayName, "-n", gatewayNamespace)
	s.Require().NoError(err, "a fresh gateway proxy pod must become Ready despite the never-ready reference")

	s.assertEventuallyServesNginx(validHost)
}

// TestControllerRestartDoesNotStrandClients restarts the control plane while
// the never-ready reference exists, then applies a new route. Under the
// reverted gates a reconnecting warm proxy could be withheld indefinitely
// (the gate re-evaluated to unsatisfiable on the fresh cache), freezing all
// config updates; now the new route must become effective.
func (s *testingSuite) TestControllerRestartDoesNotStrandClients() {
	s.assertEventuallyServesNginx(validHost)

	err := s.TestInstallation.Actions.Kubectl().RestartDeploymentAndWait(s.Ctx, "kgateway",
		"-n", s.TestInstallation.Metadata.InstallNamespace)
	s.Require().NoError(err, "can restart the kgateway controller")

	// Existing traffic keeps flowing on the proxy's retained config.
	s.assertEventuallyServesNginx(validHost)

	// A route created after the restart must reach the (reconnected, warm) proxy.
	err = s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, postrestartRouteManifest)
	s.Require().NoError(err, "can create a route after the controller restart")
	s.assertEventuallyServesNginx(postrestartHost)
}

func (s *testingSuite) assertEventuallyServesNginx(host string) {
	const timeout = 2 * time.Minute
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		statusCode, body, err := s.gatewayResponse(host)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "gateway request should complete")
		g.Expect(statusCode).To(gomega.Equal(http.StatusOK), "gateway returned body: %s", body)
		g.Expect(body).To(gomega.ContainSubstring(testdefaults.NginxResponse))
	}).WithContext(s.Ctx).WithTimeout(timeout).WithPolling(time.Second).Should(gomega.Succeed())
}

// assertEventuallyStatusAtLeast asserts the host answers with an error status
// >= the given code (e.g. 500/503 for a cluster that can never become ready).
func (s *testingSuite) assertEventuallyStatusAtLeast(host string, status int, timeout time.Duration) {
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		statusCode, body, err := s.gatewayResponse(host)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "gateway request should complete")
		g.Expect(statusCode).To(gomega.BeNumerically(">=", status), "gateway returned body: %s", body)
	}).WithContext(s.Ctx).WithTimeout(timeout).WithPolling(time.Second).Should(gomega.Succeed())
}

func (s *testingSuite) gatewayResponse(host string) (int, string, error) {
	curlResponse, err := s.TestInstallation.ClusterContext.Cli.CurlFromPod(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
		curl.WithHostHeader(host),
		curl.WithPort(8080),
		curl.WithPath("/"),
		curl.WithConnectionTimeout(2),
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
