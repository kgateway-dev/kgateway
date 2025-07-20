package metrics

import (
	"context"
	"net/http"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	e2edefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is a suite of basic control plane metrics.
type testingSuite struct {
	*base.BaseTestingSuite
}

// NewTestingSuite creates a new testing suite for control plane metrics.
func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

func (s *testingSuite) checkPodsRunning() {
	s.TestInstallation.Assertions.EventuallyPodsRunning(s.Ctx, nginxPod.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=nginx",
	})
	s.TestInstallation.Assertions.EventuallyPodsRunning(s.Ctx, proxyObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=gw1",
	})
	s.TestInstallation.Assertions.EventuallyPodsRunning(s.Ctx, kgatewayMetricsObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=kgateway",
	})
}

func (s *testingSuite) TestMetrics() {
	// Make sure pods are running.
	s.checkPodsRunning()
	// Verify the test services are created and working.
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		e2edefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
			curl.WithHostHeader("example.com"),
			curl.WithPort(8080),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring(e2edefaults.NginxResponse),
		})

	// Verify the control plane metrics are generating as expected.
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		e2edefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(kgatewayMetricsObjectMeta)),
			curl.WithPort(9092),
			curl.WithPath("/metrics"),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body: gomega.And(
				gomega.MatchRegexp(`kgateway_collection_transform_duration_seconds_count\{collection=\"Gateways\"\} \d+`),
				gomega.MatchRegexp(`kgateway_collection_transforms_total\{collection=\"Gateways\",result=\"success\"\} \d+`),
				gomega.MatchRegexp(`kgateway_collection_transforms_running\{collection=\"Gateways\"} \d+`),
				gomega.MatchRegexp(`kgateway_collection_resources\{collection=\"Gateways\",name=\"gw1\",namespace=\"default\",resource=\"Gateway\"\} \d+`),
				gomega.MatchRegexp(`kgateway_controller_reconcile_duration_seconds_count\{controller=\"gateway\"\} \d+`),
				gomega.MatchRegexp(`kgateway_controller_reconciliations_total\{controller=\"gateway\",result=\"success\"\} \d+`),
				gomega.MatchRegexp(`kgateway_controller_reconciliations_running\{controller=\"gateway\"} \d+`),
				gomega.MatchRegexp(`kgateway_status_syncer_resources\{name=\"gw1\",namespace=\"default\",resource=\"Gateway\",syncer=\"GatewayStatusSyncer\"\} \d+`),
				gomega.MatchRegexp(`kgateway_status_syncer_status_sync_duration_seconds_count\{syncer=\"GatewayStatusSyncer\"\} \d+`),
				gomega.MatchRegexp(`kgateway_status_syncer_status_syncs_total\{result=\"success\",syncer=\"GatewayStatusSyncer\"\} \d+`),
				gomega.MatchRegexp(`kgateway_translator_translation_duration_seconds_count\{translator=\"TranslateGatewayIR\"\} \d+`),
				gomega.MatchRegexp(`kgateway_translator_translations_total\{result=\"success\",translator=\"TranslateGatewayIR\"\} \d+`),
			),
		})
}

func (s *testingSuite) TestResourceCountingMetrics() {
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		e2edefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
			curl.WithHostHeader("example1.com"),
			curl.WithPort(8080),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring(e2edefaults.NginxResponse),
		})

	// Verify the control plane metrics are generating as expected.
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		e2edefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(kgatewayMetricsObjectMeta)),
			curl.WithPort(9092),
			curl.WithPath("/metrics"),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body: gomega.And(
				gomega.MatchRegexp(`kgateway_routing_domains\{gateway="gw1",namespace="default",port="8080"\} 5`),
				gomega.MatchRegexp(`kgateway_routing_domains\{gateway="gw1",namespace="default",port="8088"\} 2`),
				gomega.MatchRegexp(`kgateway_routing_domains\{gateway="gw1",namespace="default",port="8443"\} 2`),
				gomega.MatchRegexp(`kgateway_routing_domains\{gateway="gw2",namespace="default",port="8080"\} 3`),
				gomega.MatchRegexp(`kgateway_routing_domains\{gateway="gw2",namespace="default",port="8443"\} 3`),
				gomega.MatchRegexp(`kgateway_collection_gateway_resources\{namespace="default",resource="Gateway"\} 2`),
				gomega.MatchRegexp(`kgateway_collection_gateway_resources\{namespace="default",resource="HTTPRoute"\} 3`),
				gomega.MatchRegexp(`kgateway_collection_gateway_resources\{namespace="default",resource="XListenerSet"\} 1`),
			),
		})
}
