package virtualhost_options

import (
	"context"

	"github.com/stretchr/testify/suite"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
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

func (s *testingSuite) cleanupFunc(resources map[string]string) func() {
	return func() {
		for k, v := range resources {
			err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, v)
			s.NoError(err, "can delete "+k)
		}
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, proxyService, proxyDeployment)
	}
}

func (s *testingSuite) setup(resources map[string]string) {
	for k, v := range resources {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, v)
		s.NoError(err, "can apply "+k)
	}
}
func (s *testingSuite) TestConfigureVirtualHostOptions() {
	resources := map[string]string{
		"setupManifest":    setupManifest,
		"basicVhOManifest": basicVhOManifest,
	}

	s.T().Cleanup(s.cleanupFunc(resources))

	s.setup(resources)

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
		s.getterForMeta(&basicVirtualHostOptionMeta),
		core.Status_Accepted,
		defaults.KubeGwReporter,
	)
}

func (s *testingSuite) TestConfigureInvalidVirtualHostOptions() {
	resources := map[string]string{
		"setupManifest":    setupManifest,
		"basicVhOManifest": basicVhOManifest,
		"badVhOManifest":   badVhOManifest,
	}

	s.T().Cleanup(s.cleanupFunc(resources))

	s.setup(resources)

	// Check resources are created for Gateway
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment)

	// Check status is rejected on bad VirtualHostOption
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		s.getterForMeta(&badVirtualHostOptionMeta),
		core.Status_Rejected,
		defaults.KubeGwReporter,
	)
}

// The goal here is to test the behavior when multiple VHOs target a gateway with multiple listeners and only some
// conflict. This will generate a warning on the conflicted resource, but the VHO should be attached properly and
// options propagated for the listener.
func (s *testingSuite) TestConfigureVirtualHostOptionsWithSectionName() {
	resources := map[string]string{
		"setupManifest":          setupManifest,
		"basicVhOManifest":       basicVhOManifest,
		"extraVhOManifest":       extraVhOManifest,
		"sectionNameVhOManifest": sectionNameVhOManifest,
	}

	s.T().Cleanup(s.cleanupFunc(resources))

	s.setup(resources)

	// Check resources are created for Gateway
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment)

	// Check healthy response with added foo header to listener targeted by sectionName
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
			curl.WithPort(8080),
		},
		expectedResponseWithFooHeader)

	// Check healthy response with content-length removed to listener NOT targeted by sectionName
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
			curl.WithPort(8081),
		},
		expectedResponseWithoutContentLength)

	// Check status is accepted on VirtualHostOption with section name
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		s.getterForMeta(&sectionNameVirtualHostOptionMeta),
		core.Status_Accepted,
		defaults.KubeGwReporter,
	)
	// Check status is warning on VirtualHostOption with conflicting attachment,
	// despite being properly attached to another listener
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesWarningReasons(
		s.getterForMeta(&basicVirtualHostOptionMeta),
		[]string{"conflict with more-specific or older VirtualHostOption"},
		defaults.KubeGwReporter,
	)

	// Check status is warning on VirtualHostOption not selected for attachment
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesWarningReasons(
		s.getterForMeta(&extraVirtualHostOptionMeta),
		[]string{"conflict with more-specific or older VirtualHostOption"},
		defaults.KubeGwReporter,
	)
}

// The goal here is to test the behavior when multiple VHOs are targeting a gateway without sectionName. The expected
// behavior is that the oldest resource is used
func (s *testingSuite) TestMultipleVirtualHostOptions() {
	resources := map[string]string{
		"setupManifest":    setupManifest,
		"basicVhOManifest": basicVhOManifest,
		"extraVhOManifest": extraVhOManifest,
	}

	s.T().Cleanup(s.cleanupFunc(resources))

	s.setup(resources)

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
		s.getterForMeta(&basicVirtualHostOptionMeta),
		core.Status_Accepted,
		defaults.KubeGwReporter,
	)
	// Check status is warning on newer VirtualHostOption not selected for attachment
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesWarningReasons(
		s.getterForMeta(&extraVirtualHostOptionMeta),
		[]string{"conflict with more-specific or older VirtualHostOption"},
		defaults.KubeGwReporter,
	)
}

func (s *testingSuite) getterForMeta(meta *metav1.ObjectMeta) helpers.InputResourceGetter {
	return func() (resources.InputResource, error) {
		return s.testInstallation.ResourceClients.VirtualHostOptionClient().Read(meta.GetNamespace(), meta.GetName(), clients.ReadOpts{})
	}
}
