package virtualhost_options

import (
	"context"

	"github.com/stretchr/testify/suite"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// testingSuite is the entire Suite of tests for the "VirtualHostOptions" feature
type testingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *testingSuite) TestConfigureVirtualHostOptions() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, targetRefManifest)
		s.NoError(err, "can delete manifest")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, proxyService, proxyDeployment)
	})

	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, targetRefManifest)
	s.NoError(err, "can apply targetRefManifest")

	// Check resources are created for Gateway
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment)
	
	// Check healthy response with no content-length header
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
		},
		expectedResponseWithoutContentLength)

	// Check status is accepted on VirtualHostOption
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		s.getterForMeta(&virtualHostOptionMeta),
		core.Status_Accepted,
		"gloo-kube-gateway",
	)
}
func (s *testingSuite) TestConfigureVirtualHostOptionsWithSectionName() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, targetRefManifest)
		s.NoError(err, "can delete targetRefManifest")
		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, extraVhOManifest)
		s.NoError(err, "can delete extraVhOManifest")
		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, sectionNameVhOManifest)
		s.NoError(err, "can delete sectionNameVhOManifest")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, proxyService, proxyDeployment)
	})

	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, targetRefManifest)
	s.NoError(err, "can apply targetRefManifest")
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, extraVhOManifest)
	s.NoError(err, "can apply extraVhOManifest")
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, sectionNameVhOManifest)
	s.NoError(err, "can apply sectionNameVhOManifest")

	// Check resources are created for Gateway
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment)

	// Check healthy response with added foo header
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
		},
		expectedResponseWithFooHeader)

	// Check status is accepted on VirtualHostOption with section name
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		s.getterForMeta(&sectionNameVirtualHostOptionMeta),
		core.Status_Accepted,
		"gloo-kube-gateway",
	)
	// Check status is warning on VirtualHostOptions not selected for attachment
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesWarningReasons(
		s.getterForMeta(&virtualHostOptionMeta),
		[]string{"conflict with more-specific or older VirtualHostOption"},
		"gloo-kube-gateway",
	)
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesWarningReasons(
		s.getterForMeta(&extraVirtualHostOptionMeta),
		[]string{"conflict with more-specific or older VirtualHostOption"},
		"gloo-kube-gateway",
	)
}
func (s *testingSuite) TestMultipleVirtualHostOptions() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, targetRefManifest)
		s.NoError(err, "can delete targetRefManifest")
		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, extraVhOManifest)
		s.NoError(err, "can delete extraVhOManifest")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, proxyService, proxyDeployment)
	})

	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, targetRefManifest)
	s.NoError(err, "can apply targetRefManifest")
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, extraVhOManifest)
	s.NoError(err, "can apply extraVhOManifest")

	// Check resources are created for Gateway
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment)

	// Check healthy response with no content-length header
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
		},
		expectedResponseWithoutContentLength)

	// Check status is accepted on older VirtualHostOption
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		s.getterForMeta(&virtualHostOptionMeta),
		core.Status_Accepted,
		"gloo-kube-gateway",
	)
	// Check status is warning on VirtualHostOptions not selected for attachment
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesWarningReasons(
		s.getterForMeta(&extraVirtualHostOptionMeta),
		[]string{"conflict with more-specific or older VirtualHostOption"},
		"gloo-kube-gateway",
	)
}

// TODO(jbohanon) add negative test
// TODO(jbohanon) add test for multiple vhopts targeting valid gateways/listeners as well as unattached

func (s *testingSuite) getterForMeta(meta *metav1.ObjectMeta) helpers.InputResourceGetter {
	return func() (resources.InputResource, error) {
		return s.testInstallation.ResourceClients.VirtualHostOptionClient().Read(meta.GetNamespace(), meta.GetName(), clients.ReadOpts{})
	}
}
