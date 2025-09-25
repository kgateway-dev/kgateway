package timeoutretry

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/envoyutils/admincli"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
)

const (
	upstreamReqTimeout = "upstream request timeout"
)

type testingSuite struct {
	suite.Suite
	ctx             context.Context
	ti              *e2e.TestInstallation
	commonManifests []string
	testManifests   map[string][]string
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:             ctx,
		ti:              testInst,
		commonManifests: []string{setupManifest, testdefaults.CurlPodManifest, testdefaults.HttpbinManifest},
		testManifests:   map[string][]string{},
	}
}

func (s *testingSuite) SetupSuite() {
	for _, manifest := range s.commonManifests {
		err := s.ti.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err, manifest)
	}

	s.ti.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.CurlPod.Namespace, metav1.ListOptions{
		LabelSelector: testdefaults.CurlPodLabelSelector,
	})
	s.ti.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.HttpbinDeployment.Namespace, metav1.ListOptions{
		LabelSelector: testdefaults.HttpbinLabelSelector,
	})
	s.ti.Assertions.EventuallyPodsRunning(s.ctx, gatewayObjectMeta.Namespace, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=" + gatewayObjectMeta.Name,
	})
}

func (s *testingSuite) TearDownSuite() {
	for i := len(s.commonManifests) - 1; i >= 0; i-- {
		manifest := s.commonManifests[i]
		err := s.ti.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
		s.NoError(err, manifest)
	}
}

func (s *testingSuite) BeforeTest(suiteName, testName string) {
	for _, manifest := range s.testManifests[testName] {
		err := s.ti.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err, manifest)
	}
}

func (s *testingSuite) AfterTest(suiteName, testName string) {
	for _, manifest := range s.testManifests[testName] {
		err := s.ti.Actions.Kubectl().DeleteFile(s.ctx, manifest, "--grace-period", "0")
		s.NoError(err, manifest)
	}
}

func (s *testingSuite) TestRouteTimeout() {
	s.ti.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayObjectMeta)),
			curl.WithPort(8080),
			curl.WithPath("/delay/1"),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusGatewayTimeout,
			Body:       "upstream request timeout",
		},
	)
}

func (s *testingSuite) TestRetries() {
	s.ti.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayObjectMeta)),
			curl.WithPort(8080),
			curl.WithPath("/status/490"),
		},
		&testmatchers.HttpResponse{
			StatusCode: 490,
		},
	)
	// Assert that there were 2 retry attempts
	s.ti.Assertions.AssertEnvoyAdminApi(
		s.T().Context(),
		gatewayObjectMeta,
		assertStat(s.Assert(), "cluster.kube_default_httpbin_8000.upstream_rq_retry$", 2),
	)

	s.ti.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayObjectMeta)),
			curl.WithPort(8080),
			curl.WithPath("/delay/2"),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusGatewayTimeout,
			Body:       upstreamReqTimeout,
		},
	)
	// Assert that there were 2 more retry attempts, 4 in total
	s.ti.Assertions.AssertEnvoyAdminApi(
		s.T().Context(),
		gatewayObjectMeta,
		assertStat(s.Assert(), "cluster.kube_default_httpbin_8000.upstream_rq_retry$", 4),
	)

	// Test retry policy attached to Gateway's listener
	s.ti.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayObjectMeta)),
			curl.WithPort(8080),
			curl.WithPath("/status/517"),
		},
		&testmatchers.HttpResponse{
			StatusCode: 517,
		},
	)
	// Assert that there were 2 more retry attempts, 6 in total
	s.ti.Assertions.AssertEnvoyAdminApi(
		s.T().Context(),
		gatewayObjectMeta,
		assertStat(s.Assert(), "cluster.kube_default_httpbin_8000.upstream_rq_retry$", 6),
	)
}

func assertStat(a *assert.Assertions, statRegex string, val int) func(ctx context.Context, adminClient *admincli.Client) {
	return func(ctx context.Context, adminClient *admincli.Client) {
		stats, err := adminClient.GetStats(ctx, map[string]string{
			"filter": statRegex,
		})
		a.NoError(err)
		a.NotEmpty(stats)
		parts := strings.Split(stats, ":")
		a.Len(parts, 2)
		countStr := strings.TrimSpace(parts[1])
		count, err := strconv.Atoi(countStr)
		a.NoError(err)
		a.GreaterOrEqual(count, val)
	}
}
