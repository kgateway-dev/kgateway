package basicrouting

import (
	"context"
	"net/http"

	"github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	testmatchers "github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	testdefaults "github.com/solo-io/gloo/test/kubernetes/e2e/defaults"
	ossvalidation "github.com/solo-io/gloo/test/kubernetes/e2e/features/validation"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ e2e.NewSuiteFunc = NewBasicEdgeRoutingSuite

// edgeBasicRoutingSuite is the Suite of happy path tests for the basic routing cases for edge gateway resources (VirtualService, Upstream, etc.)
type edgeBasicRoutingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation
}

func NewBasicEdgeRoutingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &edgeBasicRoutingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *edgeBasicRoutingSuite) TestBasicVirtualServiceRouting() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, testdefaults.NginxPodManifest)
		s.Assertions.NoError(err, "can delete nginx pod")

		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, testdefaults.CurlPodManifest)
		s.Assertions.NoError(err, "can delete curl pod")

		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, ossvalidation.ExampleVS, "-n", s.testInstallation.Metadata.InstallNamespace)
		s.Assertions.NoError(err, "can delete virtual service")

		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, ossvalidation.ExampleUpstream, "-n", s.testInstallation.Metadata.InstallNamespace)
		s.Assertions.NoError(err, "can delete upstream")
	})

	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, testdefaults.NginxPodManifest)
	s.Assert().NoError(err)
	// Check that test resources are running
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.NginxPod.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=nginx",
	})

	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, testdefaults.CurlPodManifest)
	s.Assert().NoError(err)
	// Check that test resources are running
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.CurlPod.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=curl",
	})

	// Upstream is only rejected when the upstream plugin is run when a valid cluster is present
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, ossvalidation.ExampleUpstream, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err, "can apply valid upstream")
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		func() (resources.InputResource, error) {
			return s.testInstallation.ResourceClients.UpstreamClient().Read(s.testInstallation.Metadata.InstallNamespace, ossvalidation.ExampleUpstreamName, clients.ReadOpts{Ctx: s.ctx})
		},
		core.Status_Accepted,
		defaults.GlooReporter,
	)
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, ossvalidation.ExampleVS, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err, "can apply valid virtual service")
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		func() (resources.InputResource, error) {
			return s.testInstallation.ResourceClients.VirtualServiceClient().Read(s.testInstallation.Metadata.InstallNamespace, ossvalidation.ExampleVsName, clients.ReadOpts{Ctx: s.ctx})
		},
		core.Status_Accepted,
		defaults.GlooReporter,
	)

	// Should have a successful response
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		kubectl.PodExecOptions{
			Name:      testdefaults.CurlPod.Name,
			Namespace: testdefaults.CurlPod.Namespace,
			Container: "curl",
		},
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{
				Name:      gatewaydefaults.GatewayProxyName,
				Namespace: s.testInstallation.Metadata.InstallNamespace,
			})),
			curl.WithHostHeader("example.com"),
			curl.WithPort(80),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring(testdefaults.NginxResponse),
		})

}
