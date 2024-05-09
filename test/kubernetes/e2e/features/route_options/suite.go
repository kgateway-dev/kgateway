package route_options

import (
	"context"
	"strings"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	testdefaults "github.com/solo-io/gloo/test/kubernetes/e2e/defaults"
)

// testingSuite is the entire Suite of tests for the "Route Options" feature
type testingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation

	// maps test name to a list of manifests to apply before the test
	manifests map[string][]string
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *testingSuite) SetupSuite() {
	// We include tests with manual setup here because the cleanup is still automated via AfterTest
	s.manifests = map[string][]string{
		"TestConfigureRouteOptionsWithTargetRef":                          {setupManifest, httproute1Manifest, basicRtoTargetRefManifest},
		"TestConfigureRouteOptionsWithFilterExtension":                    {setupManifest, basicRtoManifest, httproute1ExtensionManifest},
		"TestConfigureInvalidRouteOptionsWithTargetRef":                   {setupManifest, httproute1Manifest, badRtoTargetRefManifest},
		"TestConfigureInvalidRouteOptionsWithFilterExtension":             {setupManifest, httproute1BadExtensionManifest, badRtoManifest},
		"TestConfigureRouteOptionsWithMultipleTargetRefManualSetup":       {setupManifest, httproute1Manifest, basicRtoTargetRefManifest, extraRtoTargetRefManifest},
		"TestConfigureRouteOptionsWithMultipleFilterExtensionManualSetup": {setupManifest, httproute1MultipleExtensionsManifest, basicRtoManifest, extraRtoManifest},
		"TestConfigureRouteOptionsWithTargetRefAndFilterExtension":        {setupManifest, httproute1ExtensionManifest, basicRtoManifest, extraRtoTargetRefManifest},
	}
}

func (s *testingSuite) TearDownSuite() {}

func (s *testingSuite) BeforeTest(suiteName, testName string) {
	if strings.Contains(testName, "ManualSetup") {
		return
	}

	manifests, ok := s.manifests[testName]
	if !ok {
		s.FailNow("no manifests found for %s, manifest map contents: %v", testName, s.manifests)
	}

	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.NoError(err, "can apply "+manifest)
	}
}

func (s *testingSuite) AfterTest(suiteName, testName string) {
	manifests, ok := s.manifests[testName]
	if !ok {
		s.Fail("no manifests found for " + testName)
	}

	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, manifest)
		s.NoError(err, "can delete "+manifest)
	}
}

func (s *testingSuite) TestConfigureRouteOptionsWithTargetRef() {
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
		},
		expectedResponseWithBasicTargetRefHeader)

	// Check status is accepted on RouteOption
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		s.getterForMeta(&basicRtoTargetRefMeta),
		core.Status_Accepted,
		defaults.KubeGatewayReporter,
	)
}

func (s *testingSuite) TestConfigureRouteOptionsWithFilterExtension() {
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
		},
		expectedResponseWithBasicHeader)

	// TODO(npolshak): Statuses are not supported for filter extensions yet
}

func (s *testingSuite) TestConfigureInvalidRouteOptionsWithTargetRef() {
}

func (s *testingSuite) TestConfigureInvalidRouteOptionsWithFilterExtension() {
}

// will fail until manual setup added
func (s *testingSuite) TestConfigureRouteOptionsWithMultipleTargetRefManualSetup() {
}

// will fail until manual setup added
func (s *testingSuite) TestConfigureRouteOptionsWithMultipleFilterExtensionManualSetup() {
}

func (s *testingSuite) TestConfigureRouteOptionsWithTargetRefAndFilterExtension() {
}

func (s *testingSuite) getterForMeta(meta *metav1.ObjectMeta) helpers.InputResourceGetter {
	return func() (resources.InputResource, error) {
		return s.testInstallation.ResourceClients.RouteOptionClient().Read(meta.GetNamespace(), meta.GetName(), clients.ReadOpts{})
	}
}
