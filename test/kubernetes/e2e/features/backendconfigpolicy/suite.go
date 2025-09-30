package backendconfigpolicy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	adminv3 "github.com/envoyproxy/go-control-plane/envoy/admin/v3"
	envoy_upstreams_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/envoyutils/admincli"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite defines the test suite for BackendConfigPolicy e2e tests
type testingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of kgateway
	testInstallation *e2e.TestInstallation

	// manifests maps test name to a list of manifests to apply before the test
	manifests map[string][]string
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *testingSuite) SetupSuite() {
	// define which manifests are applied for each test
	s.manifests = map[string][]string{
		"TestBackendConfigPolicy": {
			testdefaults.CurlPodManifest,
			setupManifest,
		},
		"TestBackendConfigPolicyTLSInsecureSkipVerify": {
			testdefaults.CurlPodManifest,
			tlsInsecureManifest,
			nginxManifest,
		},
		"TestBackendConfigPolicySimpleTLS": {
			testdefaults.CurlPodManifest,
			simpleTLSManifest,
			nginxManifest,
		},
		"TestBackendConfigPolicyTLSSystemCA": {
			testdefaults.CurlPodManifest,
			nginxManifest,
			systemCAManifest,
		},
		"TestBackendConfigPolicyOutlierDetection": {
			testdefaults.CurlPodManifest,
			setupManifest,
			outlierDetectionManifest,
		},
	}
}

func (s *testingSuite) TearDownSuite() {
	// nothing specific; each test cleans up via AfterTest
}

func (s *testingSuite) BeforeTest(suiteName, testName string) {
	manifests, ok := s.manifests[testName]
	if !ok {
		s.FailNow("no manifests found for %s, manifest map contents: %v", testName, s.manifests)
	}

	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Assert().NoError(err, "can apply manifest "+manifest)
	}

	// wait for common resources for all tests
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: testdefaults.CurlPodLabelSelector,
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, proxyObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=gw",
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, nginxPod.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=nginx",
	})
}

func (s *testingSuite) AfterTest(suiteName, testName string) {
	manifests, ok := s.manifests[testName]
	if !ok {
		s.FailNow("no manifests found for " + testName)
	}
	for _, manifest := range manifests {
		output, err := s.testInstallation.Actions.Kubectl().DeleteFileWithOutput(s.ctx, manifest)
		s.testInstallation.Assertions.ExpectObjectDeleted(manifest, err, output)
	}
}

func (s *testingSuite) TestBackendConfigPolicy() {
	// make sure pods are running
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, nginxPod.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=nginx",
	})

	// Should have a successful response
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
			curl.WithHostHeader("example.com"),
			curl.WithPort(8080),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring(testdefaults.NginxResponse),
		},
	)

	// envoy config should reflect the backend config policy
	s.testInstallation.Assertions.AssertEnvoyAdminApi(s.ctx, proxyObjectMeta, func(ctx context.Context, adminClient *admincli.Client) {
		s.testInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
			clusters, err := adminClient.GetDynamicClusters(ctx)
			g.Expect(err).NotTo(gomega.HaveOccurred(), "can get dynamic clusters from config dump")
			g.Expect(clusters).NotTo(gomega.BeEmpty())

			cluster, ok := clusters["kube_default_example-svc_8080"]
			g.Expect(ok).To(gomega.BeTrue(), "cluster should be in list")
			g.Expect(cluster).NotTo(gomega.BeNil())
			g.Expect(cluster.PerConnectionBufferLimitBytes.Value).To(gomega.Equal(uint32(1024)))
			g.Expect(cluster.ConnectTimeout.Seconds).To(gomega.Equal(int64(5)))

			cfg, ok := cluster.GetTypedExtensionProtocolOptions()["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"]
			g.Expect(ok).To(gomega.BeTrue(), "http protocol options should be on cluster")
			g.Expect(cfg).NotTo(gomega.BeNil())

			httpProtocolOptions := &envoy_upstreams_v3.HttpProtocolOptions{}
			err = anypb.UnmarshalTo(cfg, httpProtocolOptions, proto.UnmarshalOptions{})
			g.Expect(err).NotTo(gomega.HaveOccurred(), "can unmarshal http protocol options")

			g.Expect(httpProtocolOptions.CommonHttpProtocolOptions.IdleTimeout.Seconds).To(gomega.Equal(int64(10)))
			g.Expect(httpProtocolOptions.CommonHttpProtocolOptions.MaxHeadersCount.Value).To(gomega.Equal(uint32(15)))
			g.Expect(httpProtocolOptions.CommonHttpProtocolOptions.MaxStreamDuration.Seconds).To(gomega.Equal(int64(30)))
			g.Expect(httpProtocolOptions.CommonHttpProtocolOptions.MaxRequestsPerConnection.Value).To(gomega.Equal(uint32(100)))

			// check that a BackendConfigPolicy for HTTP2 backend is applied
			// when only CommonHttpProtocolOptions is set
			h2cCluster, ok := clusters["kube_default_httpbin-h2c_8080"]
			g.Expect(ok).To(gomega.BeTrue(), "cluster should be in list")
			g.Expect(h2cCluster).NotTo(gomega.BeNil())

			cfg, ok = h2cCluster.GetTypedExtensionProtocolOptions()["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"]
			g.Expect(ok).To(gomega.BeTrue(), "http protocol options should be on cluster")
			g.Expect(cfg).NotTo(gomega.BeNil())

			http2ProtocolOptions := &envoy_upstreams_v3.HttpProtocolOptions{}
			err = anypb.UnmarshalTo(cfg, http2ProtocolOptions, proto.UnmarshalOptions{})
			g.Expect(err).NotTo(gomega.HaveOccurred(), "can unmarshal http protocol options")

			g.Expect(http2ProtocolOptions.CommonHttpProtocolOptions.IdleTimeout.Seconds).To(gomega.Equal(int64(12)))
			g.Expect(http2ProtocolOptions.CommonHttpProtocolOptions.MaxHeadersCount.Value).To(gomega.Equal(uint32(17)))
			g.Expect(http2ProtocolOptions.CommonHttpProtocolOptions.MaxStreamDuration.Seconds).To(gomega.Equal(int64(32)))
			g.Expect(http2ProtocolOptions.CommonHttpProtocolOptions.MaxRequestsPerConnection.Value).To(gomega.Equal(uint32(102)))

			// check that a BackendConfigPolicy for HTTP1 backend is applied
			// when only CommonHttpProtocolOptions is set
			http1Cluster, ok := clusters["kube_default_httpbin_8080"]
			g.Expect(ok).To(gomega.BeTrue(), "cluster should be in list")
			g.Expect(http1Cluster).NotTo(gomega.BeNil())

			cfg, ok = http1Cluster.GetTypedExtensionProtocolOptions()["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"]
			g.Expect(ok).To(gomega.BeTrue(), "http protocol options should be on cluster")
			g.Expect(cfg).NotTo(gomega.BeNil())

			http1ProtocolOptions := &envoy_upstreams_v3.HttpProtocolOptions{}
			err = anypb.UnmarshalTo(cfg, http1ProtocolOptions, proto.UnmarshalOptions{})
			g.Expect(err).NotTo(gomega.HaveOccurred(), "can unmarshal http protocol options")

			g.Expect(http1ProtocolOptions.CommonHttpProtocolOptions.IdleTimeout.Seconds).To(gomega.Equal(int64(11)))
			g.Expect(http1ProtocolOptions.CommonHttpProtocolOptions.MaxHeadersCount.Value).To(gomega.Equal(uint32(16)))
			g.Expect(http1ProtocolOptions.CommonHttpProtocolOptions.MaxStreamDuration.Seconds).To(gomega.Equal(int64(31)))
			g.Expect(http1ProtocolOptions.CommonHttpProtocolOptions.MaxRequestsPerConnection.Value).To(gomega.Equal(uint32(101)))
		}).
			WithContext(ctx).
			WithTimeout(time.Second * 10).
			WithPolling(time.Millisecond * 200).
			Should(gomega.Succeed())
	})
}

func (s *testingSuite) TestBackendConfigPolicyTLSInsecureSkipVerify() {
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPath("/"),
			curl.WithPort(8080),
			curl.WithHeadersOnly(),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
		},
	)
}

func (s *testingSuite) TestBackendConfigPolicySimpleTLS() {
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPath("/"),
			curl.WithPort(8080),
			curl.WithHeadersOnly(),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
		},
	)
}

func (s *testingSuite) TestBackendConfigPolicyTLSSystemCA() {
	// self-signed upstream should fail when using system CA certificates
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
			curl.WithPort(8080),
			curl.WithHeadersOnly(),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusServiceUnavailable, // 503 expected on TLS validation failure
		},
	)
}

func (s *testingSuite) TestBackendConfigPolicyOutlierDetection() {
	// This test assumes that the `outlierDetectionManifest` sets up a
	// deployment with two httpbin pods, and we always ask them to respond with
	// HTTP 503. OutlierDetection::MaxEjectionPercent will therefore govern how
	// many backends are rejected. We use the 'stats' API in Envoy to verify
	// that rejection functions as expected.

	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: testdefaults.CurlPodLabelSelector,
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, nginxPod.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=nginx",
	})
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, httpbinDeployment)
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, httpbinDeployment.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=httpbin",
	})

	// Send enough requests to trigger outlier detection (see `Consecutive5xx`)
	numOkRequests := 10
	for i := range 2 * numOkRequests {
		expectedStatusCode := 503
		if i < numOkRequests {
			// Let's not go into panic mode (see `lb_healthy_panic`). This may
			// be unnecessary, but is more realistic than a service that never
			// had any success.
			expectedStatusCode = 200
		}
		path := fmt.Sprintf("/status/%v", expectedStatusCode)
		s.testInstallation.Assertions.AssertEventualCurlResponse(
			s.ctx,
			testdefaults.CurlPodExecOpt,
			[]curl.Option{
				curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
				curl.WithHostHeader("httpbin.example.com"),
				curl.WithPort(8080),
				curl.WithPath(path),
			},
			&testmatchers.HttpResponse{
				StatusCode: expectedStatusCode,
			},
		)
		time.Sleep(10 * time.Millisecond) // see also OutlierDetection.Interval
	}

	// envoy must have time to eject, so let's be a little slower just to
	// decrease the probability of a flaky test:
	time.Sleep(250 * time.Millisecond)

	// Check envoy stats to verify that outlier detection has ejected
	// floor(0.51 * |replicas|) = floor(1.02) = 1 backends.
	s.testInstallation.Assertions.AssertEnvoyAdminApi(s.ctx, proxyObjectMeta, func(ctx context.Context, adminClient *admincli.Client) {
		s.testInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
			metricPrefix := "cluster.kube_default_httpbin_8080"
			out, err := adminClient.GetStats(ctx, map[string]string{
				// see https://www.envoyproxy.io/docs/envoy/latest/operations/admin#get--stats
				"format": "json",
				"filter": fmt.Sprintf("^%s.outlier_detection.ejections_total", metricPrefix),
			})
			g.Expect(err).NotTo(gomega.HaveOccurred(), "can get envoy stats")

			var resp map[string][]adminv3.SimpleMetric
			err = json.Unmarshal([]byte(out), &resp)
			g.Expect(err).NotTo(gomega.HaveOccurred(), "can unmarshal envoy stats response")

			stats := resp["stats"]
			g.Expect(stats).To(gomega.HaveLen(1), "expected 1 matching stats result")
			g.Expect(stats[0].GetName()).To(gomega.HavePrefix(metricPrefix))
			g.Expect(stats[0].GetValue()).To(gomega.Equal(uint64(1)))
			// If this fails, be aware of `lb_healthy_panic`.
		}).WithTimeout(20 * time.Second).WithPolling(time.Second).Should(gomega.Succeed())
	})
}
