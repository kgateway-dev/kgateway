package backendconfigpolicy

import (
	"context"
	"net/http"
	"time"

	envoy_upstreams_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envoyutils/admincli"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

type testingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of kgateway
	testInstallation *e2e.TestInstallation
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *testingSuite) TestBackendConfigPolicy() {
	manifests := []string{
		testdefaults.CurlPodManifest,
		setupManifest,
	}
	manifestObjects := []client.Object{
		testdefaults.CurlPod, // curl
		nginxPod, exampleSvc, // nginx
		proxyService, proxyServiceAccount, proxyDeployment, // proxy
	}

	s.T().Cleanup(func() {
		for _, manifest := range manifests {
			err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
			s.Require().NoError(err)
		}
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, manifestObjects...)
	})

	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err)
	}
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, manifestObjects...)

	// make sure pods are running
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: testdefaults.CurlPodLabelSelector,
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, nginxPod.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=nginx",
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, proxyObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=gw",
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
	manifests := []string{
		testdefaults.CurlPodManifest,
		tlsInsecureManifest,
		nginxManifest,
	}
	manifestObjects := []client.Object{
		testdefaults.CurlPod,                               // curl
		proxyService, proxyServiceAccount, proxyDeployment, // proxy
		nginxPod,
	}

	s.T().Cleanup(func() {
		for _, manifest := range manifests {
			err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
			s.Require().NoError(err)
		}
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, manifestObjects...)
	})

	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err)
	}
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, manifestObjects...)

	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: testdefaults.CurlPodLabelSelector,
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, nginxPod.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=nginx",
	})

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
	manifests := []string{
		testdefaults.CurlPodManifest,
		simpleTLSManifest,
		nginxManifest,
	}
	manifestObjects := []client.Object{
		testdefaults.CurlPod,                               // curl
		proxyService, proxyServiceAccount, proxyDeployment, // proxy
		nginxPod,
	}

	s.T().Cleanup(func() {
		for _, manifest := range manifests {
			err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
			s.Require().NoError(err)
		}
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, manifestObjects...)
	})

	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err)
	}
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, manifestObjects...)

	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: testdefaults.CurlPodLabelSelector,
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, nginxPod.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=nginx",
	})

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
