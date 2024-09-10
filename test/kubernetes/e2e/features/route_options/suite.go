package route_options

import (
	"context"
	"net/http"
	"strings"

	. "github.com/onsi/gomega"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	testdefaults "github.com/solo-io/gloo/test/kubernetes/e2e/defaults"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

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

func NewTestingSuite(
	ctx context.Context,
	testInst *e2e.TestInstallation,
) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *testingSuite) SetupSuite() {
	// Check that the common setup manifest is applied
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, setupManifest)
	s.NoError(err, "can apply "+setupManifest)
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment, exampleSvc, nginxPod)
	// Check that test resources are running
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, nginxPod.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=nginx",
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, proxyDeployment.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=gloo-proxy-gw",
	})

	// We include tests with manual setup here because the cleanup is still automated via AfterTest
	s.manifests = map[string][]string{
		"TestConfigureRouteOptionsWithTargetRef":                          {httproute1Manifest, basicRtoTargetRefManifest},
		"TestConfigureRouteOptionsWithFilterExtension":                    {basicRtoManifest, httproute1ExtensionManifest},
		"TestConfigureInvalidRouteOptionsWithTargetRef":                   {httproute1Manifest, badRtoTargetRefManifest},
		"TestConfigureInvalidRouteOptionsWithFilterExtension":             {httproute1BadExtensionManifest, badRtoManifest},
		"TestConfigureRouteOptionsWithMultipleTargetRefManualSetup":       {httproute1Manifest, basicRtoTargetRefManifest, extraRtoTargetRefManifest},
		"TestConfigureRouteOptionsWithMultipleFilterExtensionManualSetup": {httproute1MultipleExtensionsManifest, basicRtoManifest, extraRtoManifest},
		"TestConfigureRouteOptionsWithTargetRefAndFilterExtension":        {httproute1ExtensionManifest, basicRtoManifest, extraRtoTargetRefManifest},
		"TestOptionsMerge": {mergeManifest},
	}
}

func (s *testingSuite) TearDownSuite() {
	// Delete the common setup manifest
	err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, setupManifest)
	s.NoError(err, "can delete "+setupManifest)
}

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
		s.FailNow("no manifests found for " + testName)
	}

	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, manifest)
		s.NoError(err, "can delete "+manifest)
	}
}

func (s *testingSuite) TestConfigureRouteOptionsWithTargetRef() {
	s.assertEventuallyCurlRespondsWith(expectedResponseWithBasicTargetRefHeader)

	// Check status is accepted on RouteOption
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		s.getterForMeta(&basicRtoTargetRefMeta),
		core.Status_Accepted,
		defaults.KubeGatewayReporter,
	)
}

func (s *testingSuite) TestConfigureRouteOptionsWithFilterExtension() {
	s.assertEventuallyCurlRespondsWith(expectedResponseWithBasicHeader)

	// TODO(npolshak): Statuses are not supported for filter extensions yet
}

func (s *testingSuite) TestConfigureInvalidRouteOptionsWithTargetRef() {
	// Check status is rejected on RouteOption
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		s.getterForMeta(&badRtoTargetRefMeta),
		core.Status_Rejected,
		defaults.KubeGatewayReporter,
	)
}

// TODO(jbohanon) statuses not implemented for extension ref
// func (s *testingSuite) TestConfigureInvalidRouteOptionsWithFilterExtension() {
// 	// Check status is rejected on RouteOption
// 	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
// 		s.getterForMeta(&badRtoMeta),
// 		core.Status_Rejected,
// 		defaults.KubeGatewayReporter,
// 	)
// }

// will fail until manual setup added
func (s *testingSuite) TestConfigureRouteOptionsWithMultipleTargetRefManualSetup() {
	// Manually apply our manifests so we can assert that basic rto exists before applying extra rto.
	// This is needed because our solo-kit clients currently do not return creationTimestamp
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, httproute1Manifest)
	s.NoError(err, "can apply "+httproute1Manifest)

	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, basicRtoTargetRefManifest)
	s.NoError(err, "can apply "+basicRtoTargetRefManifest)
	// Check status is accepted before moving on to apply conflicting rto
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		s.getterForMeta(&basicRtoTargetRefMeta),
		core.Status_Accepted,
		defaults.KubeGatewayReporter,
	)

	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, extraRtoTargetRefManifest)
	s.NoError(err, "can apply "+extraRtoTargetRefManifest)

	// TODO(jbohanon) discuss the appropriate status for RouteOptions which are not attached
	// // Check status is accepted on conflicted RouteOption. RouteOptions do not currently
	// // produce warnings on conflicts like VirtualHostOptions do
	// s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
	// 	s.getterForMeta(&extraRtoTargetRefMeta),
	// 	core.Status_Accepted,
	// 	defaults.KubeGatewayReporter,
	// )

	// Check status is still accepted on attached RouteOption.
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		s.getterForMeta(&basicRtoTargetRefMeta),
		core.Status_Accepted,
		defaults.KubeGatewayReporter,
	)
	// TODO(jbohanon) figure out how to check tracking the source and/or add warnings to
	// conflicted RouteOption resources

	// make sure we are getting responses with the older RouteOption applied
	s.assertEventuallyCurlRespondsWith(expectedResponseWithBasicTargetRefHeader)
}

// We currently only honor the first listed RouteOptions extension ref. To validate this in this test
// we apply the extra rto (second listed) first, then apply the basic rto (first listed) and verify
// that we are seeing behavior congruent with the basic rto.
func (s *testingSuite) TestConfigureRouteOptionsWithMultipleFilterExtensionManualSetup() {
	// Manually apply our manifests so we can assert that basic rto exists before applying extra rto.
	// This is needed because our solo-kit clients currently do not return creationTimestamp
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, httproute1MultipleExtensionsManifest)
	s.NoError(err, "can apply "+httproute1MultipleExtensionsManifest)

	// here we apply the extra manifest first so that it is older
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, extraRtoManifest)
	s.NoError(err, "can apply "+extraRtoManifest)
	// TODO(jbohanon) statuses not implemented for extension ref
	// // Check status is accepted before moving on to apply conflicting rto
	// s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
	// 	s.getterForMeta(&extraRtoMeta),
	// 	core.Status_Accepted,
	// 	defaults.KubeGatewayReporter,
	// )

	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, basicRtoManifest)
	s.NoError(err, "can apply "+basicRtoManifest)

	// TODO(jbohanon) statuses not implemented for extension ref
	// // Check status is accepted on ignored extension ref RouteOption. RouteOptions do not currently
	// // produce warnings on conflicts like VirtualHostOptions do
	// s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
	// 	s.getterForMeta(&extraRtoMeta),
	// 	core.Status_Accepted,
	// 	defaults.KubeGatewayReporter,
	// )

	// TODO(jbohanon) statuses not implemented for extension ref
	// // Check status is still accepted on attached RouteOption.
	// s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
	// 	s.getterForMeta(&basicRtoMeta),
	// 	core.Status_Accepted,
	// 	defaults.KubeGatewayReporter,
	// )

	// make sure we are getting responses with the first-listed RouteOption applied
	s.assertEventuallyCurlRespondsWith(expectedResponseWithBasicHeader)
}

func (s *testingSuite) TestConfigureRouteOptionsWithTargetRefAndFilterExtension() {
	// TODO(jbohanon) statuses not implemented for extension ref
	// Extension ref takes precedence over target ref, so check that the resource is accepted
	// s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
	// 	s.getterForMeta(&basicRtoMeta),
	// 	core.Status_Accepted,
	// 	defaults.KubeGatewayReporter,
	// )

	// TODO(jbohanon) assess options for statuses on un-attached resources
	// // Check status is still accepted on attached RouteOption.
	// s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
	// 	s.getterForMeta(&extraRtoTargetRefMeta),
	// 	core.Status_Accepted,
	// 	defaults.KubeGatewayReporter,
	// )
	// make sure we are getting responses with the extension ref RouteOption applied
	s.assertEventuallyCurlRespondsWith(expectedResponseWithBasicHeader)
}

// TestOptionsMerge tests the merging of RouteOptions targeting the same HTTPRoute
func (s *testingSuite) TestOptionsMerge() {
	// Check status is accepted on RouteOptions
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		s.getterForMeta(&extref1RtoMeta),
		core.Status_Accepted,
		defaults.KubeGatewayReporter,
	)
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		s.getterForMeta(&extref2RtoMeta),
		core.Status_Accepted,
		defaults.KubeGatewayReporter,
	)
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		s.getterForMeta(&target1RtoMeta),
		core.Status_Accepted,
		defaults.KubeGatewayReporter,
	)
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		s.getterForMeta(&target2RtoMeta),
		core.Status_Accepted,
		defaults.KubeGatewayReporter,
	)

	s.testInstallation.Assertions.AssertEventuallyConsistentCurlResponse(s.ctx, testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
			curl.WithPath("/headers"),
		},
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			// Expect:
			// x-foo: extref response header due to extref1 RouteOption
			// X-Forwarded-Host header response in body due to extref2 RouteOption
			// /anything/rewrite path rewrite due to target-1 RouteOption
			// foo.com host rewrite due to target-2 RouteOption
			//
			// ref: test/kubernetes/e2e/features/route_options/testdata/merge.yaml
			Body:    And(ContainSubstring("/anything/rewrite"), ContainSubstring("foo.com"), ContainSubstring("X-Forwarded-Host")),
			Headers: map[string]interface{}{"x-foo": Equal("extref")},
		})
}

// This helper function adds the standard format for getter construction, allowing a reader to
// more easily view what is happening in the main test
func (s *testingSuite) getterForMeta(meta *metav1.ObjectMeta) helpers.InputResourceGetter {
	return func() (resources.InputResource, error) {
		return s.testInstallation.ResourceClients.RouteOptionClient().Read(meta.GetNamespace(), meta.GetName(), clients.ReadOpts{})
	}
}

// This helper function adds the standard options passed to this assertion, allowing a reader to
// more easily view what is being asserted in the main test
func (s *testingSuite) assertEventuallyCurlRespondsWith(response *matchers.HttpResponse) {
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
		},
		response)
}
