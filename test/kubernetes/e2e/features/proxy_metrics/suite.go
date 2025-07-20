//go:build ignore

package proxy_metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	adminv3 "github.com/envoyproxy/go-control-plane/envoy/admin/v3"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"

	gloov1 "github.com/kgateway-dev/kgateway/v2/internal/gloo/pkg/api/v1"
	"github.com/kgateway-dev/kgateway/v2/internal/gloo/pkg/translator"
	"github.com/kgateway-dev/kgateway/v2/internal/gloo/pkg/upstreams/kubernetes"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envoyutils/admincli"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		base.NewBaseTestingSuite(ctx, testInst, e2e.MustTestHelper(ctx, testInst), base.SimpleTestCase{}, testCases),
	}
}

// TestKubeServiceSuccessStats sends a number of requests to a kube Service and checks that the cluster metrics
// show the expected number of successful requests
func (s *testingSuite) TestKubeServiceSuccessStats() {
	s.TestInstallation.Assertions.EventuallyReadyReplicas(s.Ctx, proxyDeployment.ObjectMeta, gomega.Equal(1))

	kubeSvcUpstream := kubernetes.ServiceToUpstream(s.Ctx, exampleSvc, exampleSvc.Spec.Ports[0])
	s.sendAndAssertNumSuccessfulRequests(3, kubeSvcUpstream)
}

// TestKubeUpstreamSuccessStats sends a number of requests to a kube Upstream and checks that the cluster metrics
// show the expected number of successful requests
func (s *testingSuite) TestKubeUpstreamSuccessStats() {
	s.TestInstallation.Assertions.EventuallyReadyReplicas(s.Ctx, proxyDeployment.ObjectMeta, gomega.Equal(1))

	s.sendAndAssertNumSuccessfulRequests(2, kubeUpstream)
}

// sendAndAssertNumSuccessfulRequests sends the specified number of requests to the given upstream
// and verifies that the upstream cluster's stats show the expected number of successful requests
func (s *testingSuite) sendAndAssertNumSuccessfulRequests(numRequests int, upstream *gloov1.Upstream) {
	// send the specified number of requests with a successful responses
	for i := 0; i < numRequests; i++ {
		s.TestInstallation.Assertions.AssertEventualCurlResponse(
			s.Ctx,
			defaults.CurlPodExecOpt,
			[]curl.Option{
				curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
				curl.WithHostHeader("example.com"),
			},
			&testmatchers.HttpResponse{
				StatusCode: http.StatusOK,
			})
	}

	// make sure the stats show the expected number of successful responses
	s.TestInstallation.Assertions.AssertEnvoyAdminApi(
		s.Ctx,
		proxyDeployment.ObjectMeta,
		metricsAssertion(s.TestInstallation,
			getUpstreamSuccessfulRequestsMetricName(upstream),
			numRequests,
		),
	)
}

// getUpstreamSuccessfulRequestsMetricName gets a metric name used for looking up the total 2xx responses from the given upstream
func getUpstreamSuccessfulRequestsMetricName(upstream *gloov1.Upstream) string {
	clusterStatName := translator.UpstreamToClusterStatsName(upstream)
	return fmt.Sprintf("cluster.%s.upstream_rq_2xx", clusterStatName)
}

// metricsAssertion asserts that the envoy admin server stats endpoint shows a metric with the given name and value
func metricsAssertion(testInstallation *e2e.TestInstallation, metricName string, expectedMetricValue int) func(ctx context.Context, adminClient *admincli.Client) {
	return func(ctx context.Context, adminClient *admincli.Client) {
		testInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
			out, err := adminClient.GetStats(ctx, map[string]string{
				// see https://www.envoyproxy.io/docs/envoy/latest/operations/admin#get--stats
				"format": "json",
				"filter": fmt.Sprintf("^%s$", metricName),
			})
			g.Expect(err).NotTo(gomega.HaveOccurred(), "can get envoy stats")

			var resp map[string][]adminv3.SimpleMetric
			err = json.Unmarshal([]byte(out), &resp)
			g.Expect(err).NotTo(gomega.HaveOccurred(), "can unmarshal envoy stats response")

			stats := resp["stats"]
			g.Expect(stats).To(gomega.HaveLen(1), "expected 1 matching stats result")
			g.Expect(stats[0].GetName()).To(gomega.Equal(metricName))
			g.Expect(stats[0].GetValue()).To(gomega.Equal(uint64(expectedMetricValue)))
		}).
			WithContext(ctx).
			WithTimeout(time.Second * 10).
			WithPolling(time.Millisecond * 200).
			Should(gomega.Succeed())
	}
}
