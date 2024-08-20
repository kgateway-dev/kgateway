package virtualhost_options

import (
	"context"
	"net/http"
	"strings"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/stretchr/testify/suite"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	testdefaults "github.com/solo-io/gloo/test/kubernetes/e2e/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is the entire Suite of tests for the "VirtualHostOptions" feature
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
	for _, manifest := range setupManifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.NoError(err, "can apply "+manifest)
	}
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment, exampleSvc, nginxPod, testdefaults.CurlPod)
	// Check that test resources are running
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, nginxPod.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=nginx",
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, proxyDeployment.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=gloo-proxy-gw",
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=curl",
	})

	// We include tests with manual setup here because the cleanup is still automated via AfterTest
	s.manifests = map[string][]string{
		"TestConfigureVirtualHostOptions":        {basicVhOManifest},
		"TestConfigureInvalidVirtualHostOptions": {basicVhOManifest, badVhOManifest},
		// Test creates the manifests to control ordering and timing of resource creation
		"TestConfigureVirtualHostOptionsWithSectionNameManualSetup": {},
		"TestMultipleVirtualHostOptionsManualSetup":                 {basicVhOManifest, extraVhOManifest},
		// Test creates the manifests to control ordering and timing of resource creation
		"TestOptionsMerge": {},
	}
}

func (s *testingSuite) TearDownSuite() {
	// Check that the common setup manifest is deleted
	for _, manifest := range setupManifests {
		output, err := s.testInstallation.Actions.Kubectl().DeleteFileWithOutput(s.ctx, manifest)
		s.NoError(err, "can delete "+manifest)
		s.testInstallation.Assertions.ExpectObjectDeleted(manifest, err, output)
	}
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
		output, err := s.testInstallation.Actions.Kubectl().ApplyFileWithOutput(s.ctx, manifest)
		s.testInstallation.Assertions.ExpectObjectAdmitted(manifest, err, output, "Validating *v1.VirtualHostOption failed")
	}
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

func (s *testingSuite) TestConfigureVirtualHostOptions() {
	// Check healthy response with no content-length header
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
		},
		expectedResponseWithoutContentLength)

	// Check status is accepted on VirtualHostOption
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		s.getterForMeta(&basicVirtualHostOptionMeta),
		core.Status_Accepted,
		defaults.KubeGatewayReporter,
	)
}

func (s *testingSuite) TestConfigureInvalidVirtualHostOptions() {
	if !s.testInstallation.Metadata.ValidationAlwaysAccept {
		s.testInstallation.Assertions.ExpectGlooObjectNotExist(
			s.ctx,
			s.getterForMeta(&badVirtualHostOptionMeta),
			&badVirtualHostOptionMeta,
		)
	} else {
		// Check status is rejected on bad VirtualHostOption
		s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
			s.getterForMeta(&badVirtualHostOptionMeta),
			core.Status_Rejected,
			defaults.KubeGatewayReporter,
		)
	}
}

// The goal here is to test the behavior when multiple VHOs target a gateway with multiple listeners and only some
// conflict. This will generate a warning on the conflicted resource, but the VHO should be attached properly and
// options propagated for the listener.
func (s *testingSuite) TestConfigureVirtualHostOptionsWithSectionNameManualSetup() {
	// Manually apply our manifests so we can assert that basic vho exists before applying extra vho.
	// This is needed because our solo-kit clients currently do not return creationTimestamp
	s.T().Cleanup(func() {
		output, err := s.testInstallation.Actions.Kubectl().DeleteFileWithOutput(s.ctx, basicVhOManifest)
		s.testInstallation.Assertions.ExpectObjectDeleted(basicVhOManifest, err, output)

		output, err = s.testInstallation.Actions.Kubectl().DeleteFileWithOutput(s.ctx, sectionNameVhOManifest)
		s.testInstallation.Assertions.ExpectObjectDeleted(sectionNameVhOManifest, err, output)

		output, err = s.testInstallation.Actions.Kubectl().DeleteFileWithOutput(s.ctx, extraVhOManifest)
		s.testInstallation.Assertions.ExpectObjectDeleted(extraVhOManifest, err, output)
	})

	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, basicVhOManifest)
	s.NoError(err, "can apply "+basicVhOManifest)
	// Check status is accepted before moving on to apply conflicting vho
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		s.getterForMeta(&basicVirtualHostOptionMeta),
		core.Status_Accepted,
		defaults.KubeGatewayReporter,
	)

	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, extraVhOManifest)
	s.NoError(err, "can apply "+extraVhOManifest)

	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, sectionNameVhOManifest)
	s.NoError(err, "can apply "+sectionNameVhOManifest)

	// Check healthy response with added foo header to listener targeted by sectionName
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
			curl.WithPort(8080),
		},
		expectedResponseWithFooHeader)

	// Check healthy response with content-length removed to listener NOT targeted by sectionName
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
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
		defaults.KubeGatewayReporter,
	)
	// Check status is warning on VirtualHostOption with conflicting attachment,
	// despite being properly attached to another listener
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesWarningReasons(
		s.getterForMeta(&basicVirtualHostOptionMeta),
		[]string{"conflict with more specific or older VirtualHostOptions"},
		defaults.KubeGatewayReporter,
	)

	// Check status is warning on VirtualHostOption not selected for attachment
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesWarningReasons(
		s.getterForMeta(&extraVirtualHostOptionMeta),
		[]string{"conflict with more specific or older VirtualHostOptions"},
		defaults.KubeGatewayReporter,
	)
}

// The goal here is to test the behavior when multiple VHOs are targeting a gateway without sectionName. The expected
// behavior is that the oldest resource is used
func (s *testingSuite) TestMultipleVirtualHostOptionsManualSetup() {
	// Manually apply our manifests so we can assert that basic vho exists before applying extra vho.
	// This is needed because our solo-kit clients currently do not return creationTimestamp
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, basicVhOManifest)
	s.NoError(err, "can apply "+basicVhOManifest)
	// Check status is accepted before moving on to apply conflicting vho
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		s.getterForMeta(&basicVirtualHostOptionMeta),
		core.Status_Accepted,
		defaults.KubeGatewayReporter,
	)

	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, extraVhOManifest)
	s.NoError(err, "can apply "+extraVhOManifest)

	// Check healthy response with no content-length header
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
		},
		expectedResponseWithoutContentLength)

	// Check status is accepted on older VirtualHostOption
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		s.getterForMeta(&basicVirtualHostOptionMeta),
		core.Status_Accepted,
		defaults.KubeGatewayReporter,
	)
	// Check status is warning on newer VirtualHostOption not selected for attachment
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesWarningReasons(
		s.getterForMeta(&extraVirtualHostOptionMeta),
		[]string{"conflict with more specific or older VirtualHostOptions"},
		defaults.KubeGatewayReporter,
	)
}

func (s *testingSuite) TestOptionsMerge() {
	s.T().Cleanup(func() {
		output, err := s.testInstallation.Actions.Kubectl().DeleteFileWithOutput(s.ctx, basicVhOManifest)
		s.testInstallation.Assertions.ExpectObjectDeleted(basicVhOManifest, err, output)

		output, err = s.testInstallation.Actions.Kubectl().DeleteFileWithOutput(s.ctx, extraVhOMergeManifest)
		s.testInstallation.Assertions.ExpectObjectDeleted(extraVhOMergeManifest, err, output)
	})

	_, err := s.testInstallation.Actions.Kubectl().ApplyFileWithOutput(s.ctx, basicVhOManifest)
	s.Require().NoError(err)
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		s.getterForMeta(&basicVirtualHostOptionMeta),
		core.Status_Accepted,
		defaults.KubeGatewayReporter,
	)

	_, err = s.testInstallation.Actions.Kubectl().ApplyFileWithOutput(s.ctx, extraVhOMergeManifest)
	s.Require().NoError(err)
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		s.getterForMeta(&extraMergeVirtualHostOptionMeta),
		core.Status_Accepted,
		defaults.KubeGatewayReporter,
	)

	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
		},
		// Expect:
		// - content-length header to be removed by basic-vho.yaml
		// - x-envoy-attempt-count header to be added by extra-vho-merge.yaml
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Custom: gomega.And(
				gomega.Not(matchers.ContainHeaderKeys([]string{"content-length"})),
				matchers.ContainHeaderKeys([]string{"x-envoy-attempt-count"}),
			),
			Body: gstruct.Ignore(),
		},
	)
}

func (s *testingSuite) getterForMeta(meta *metav1.ObjectMeta) helpers.InputResourceGetter {
	return func() (resources.InputResource, error) {
		return s.testInstallation.ResourceClients.VirtualHostOptionClient().Read(meta.GetNamespace(), meta.GetName(), clients.ReadOpts{})
	}
}
