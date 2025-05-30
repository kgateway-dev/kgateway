package backendconfigpolicy

import (
	"context"
	"net/http"

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
		clusters, err := adminClient.GetDynamicClusters(ctx)
		s.Require().NoError(err)
		s.Require().NotNil(clusters)
		s.Require().NotEmpty(clusters)

		cluster, ok := clusters["kube_default_example-svc_8080"]
		s.Assert().True(ok)
		s.Assert().NotNil(cluster)
		s.Assert().Equal(uint32(1024), cluster.PerConnectionBufferLimitBytes.Value)
		s.Assert().Equal(int64(5), cluster.ConnectTimeout.Seconds)
		cfg, ok := cluster.GetTypedExtensionProtocolOptions()["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"]
		s.Assert().True(ok)
		s.Assert().NotNil(cfg)
		http2ProtocolOptions := &envoy_upstreams_v3.HttpProtocolOptions{}
		err = anypb.UnmarshalTo(cfg, http2ProtocolOptions, proto.UnmarshalOptions{})
		s.Assert().NoError(err)
		s.Assert().Equal(int64(10), http2ProtocolOptions.CommonHttpProtocolOptions.IdleTimeout.Seconds)
		s.Assert().Equal(uint32(15), http2ProtocolOptions.CommonHttpProtocolOptions.MaxHeadersCount.Value)
		s.Assert().Equal(int64(30), http2ProtocolOptions.CommonHttpProtocolOptions.MaxStreamDuration.Seconds)
		s.Assert().Equal(uint32(100), http2ProtocolOptions.CommonHttpProtocolOptions.MaxRequestsPerConnection.Value)
	})
}
