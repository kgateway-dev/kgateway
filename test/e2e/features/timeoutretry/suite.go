//go:build e2e

package timeoutretry

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	"github.com/kgateway-dev/kgateway/v2/test/envoyutils/admincli"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

const (
	upstreamReqTimeout = "upstream request timeout"
)

type testingSuite struct {
	*base.BaseTestingSuite
}

var (
	testCases = map[string]*base.TestCase{
		"TestRouteTimeout": {},
		"TestRetries":      {},
	}
)

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases, base.WithMinGwApiVersion(base.GwApiRequireRouteNames)),
	}
}

func (s *testingSuite) TestRouteTimeout() {
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
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
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
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
	s.TestInstallation.AssertionsT(s.T()).AssertEnvoyAdminApi(
		s.T().Context(),
		gatewayObjectMeta,
		assertStat(s.Assert(), "cluster.kube_default_httpbin_8000.upstream_rq_retry$", 2),
	)

	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
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
	s.TestInstallation.AssertionsT(s.T()).AssertEnvoyAdminApi(
		s.T().Context(),
		gatewayObjectMeta,
		assertStat(s.Assert(), "cluster.kube_default_httpbin_8000.upstream_rq_retry$", 4),
	)

	// Test retry policy attached to Gateway's listener
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
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
	s.TestInstallation.AssertionsT(s.T()).AssertEnvoyAdminApi(
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
