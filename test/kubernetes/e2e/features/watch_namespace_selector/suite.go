package watch_namespace_selector

import (
	"context"
	"net/http"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/tests/base"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		base.NewBaseTestingSuite(ctx, testInst, e2e.MustTestHelper(ctx, testInst), setupSuite, testCases),
	}
}

func (s *testingSuite) SetupSuite() {
	s.BaseTestingSuite.SetupSuite()

	// Apply a VS in the install namespace
	s.applyFile(installNamespaceVSManifest, "-n", s.TestHelper.InstallNamespace)
}

func (s *testingSuite) TearDownSuite() {
	// Delete VS in the install namespace
	s.deleteFile(installNamespaceVSManifest, "-n", s.TestHelper.InstallNamespace)

	s.BaseTestingSuite.TearDownSuite()
}

func (s *testingSuite) TestMatchLabels() {
	s.testWatchNamespaceSelector()
}

func (s *testingSuite) TestMatchExpressions() {
	s.testWatchNamespaceSelector()
}

func (s *testingSuite) testWatchNamespaceSelector() {
	// Ensure the install namespace is watched even if not specified
	s.TestInstallation.Assertions.CurlEventuallyRespondsWithStatus(s.Ctx, "install-ns/", http.StatusOK)

	// Ensure CRs defined in non watched-namespaces are not translated
	s.TestInstallation.Assertions.CurlConsistentlyRespondsWithStatus(s.Ctx, "random/", http.StatusNotFound)

	s.labelSecondNamespaceAsWatched()

	// The VS defined in the random namespace should be translated
	s.TestInstallation.Assertions.CurlEventuallyRespondsWithStatus(s.Ctx, "random/", http.StatusOK)
}

func (s *testingSuite) TestUnwatchedNamespaceValidation() {
	s.applyFile(unlabeledRandomNamespaceManifest)
	s.applyFile(randomUpstreamManifest)

	// It should successfully apply inconsequential labels to a ns without validation errors
	s.addLabelToSecondNamespace()
	s.removeLabelFromSecondNamespace()

	// Deleting resources in the namespace and the namespace itself should not error out
	s.deleteFile(randomUpstreamManifest)
	s.deleteFile(unlabeledRandomNamespaceManifest)
}

func (s *testingSuite) TestWatchedNamespaceValidation() {
	s.applyFile(unlabeledRandomNamespaceManifest)
	s.applyFile(randomUpstreamManifest)

	s.labelSecondNamespaceAsWatched()

	// It should successfully apply inconsequential labels to a ns we watch without validation errors
	s.addLabelToSecondNamespace()
	s.removeLabelFromSecondNamespace()

	s.Eventually(func() bool {
		err := s.TestHelper.ApplyFile(s.Ctx, installNamespaceWithRandomUpstreamVSManifest, "-n", s.TestHelper.InstallNamespace)
		return err == nil
	}, time.Minute*2, time.Second*10)

	// The upstream defined in the random namespace should be translated and referenced
	s.TestInstallation.Assertions.CurlEventuallyRespondsWithStatus(s.Ctx, "/get", http.StatusOK)

	// Trying to unwatch the namespace that has an upstream referenced in another namespace leads to an error
	_, errOut, err := s.TestHelper.Execute(s.Ctx, "label", "ns", "random", "watch-")

	s.Contains(errOut, `admission webhook "gloo.namespace-selector.svc" denied the request: resource incompatible with current Gloo snapshot`)
	s.Contains(errOut, `Route Warning: InvalidDestinationWarning. Reason: *v1.Upstream { random.postman-echo } not found`)
	s.Error(err)

	// Trying to delete the namespace also errors out
	_, errOut, err = s.TestHelper.Execute(s.Ctx, "delete", "ns", "random")
	s.Contains(errOut, `admission webhook "gloo.namespace-selector.svc" denied the request: resource incompatible with current Gloo snapshot`)
	s.Contains(errOut, `Route Warning: InvalidDestinationWarning. Reason: *v1.Upstream { random.postman-echo } not found`)
	s.Error(err)

	// Ensure we didn't break the validation server while we're at it
	s.addLabelToSecondNamespace()
	s.removeLabelFromSecondNamespace()

	s.deleteFile(installNamespaceWithRandomUpstreamVSManifest, "-n", s.TestHelper.InstallNamespace)

	s.unwatchNamespace()

	// The upstream defined in the random namespace should be translated and referenced
	s.TestInstallation.Assertions.CurlEventuallyRespondsWithStatus(s.Ctx, "/get", http.StatusNotFound)

	// Optimists invent airplanes; pessimists invent parachutes
	s.addLabelToSecondNamespace()
	s.removeLabelFromSecondNamespace()

	s.deleteFile(randomUpstreamManifest)
	s.deleteFile(unlabeledRandomNamespaceManifest)
}

func (s *testingSuite) labelSecondNamespaceAsWatched() {
	// Label the `random` namespace with the watchNamespaceSelector labels
	// kubectl label ns random watch=this
	out, _, err := s.TestHelper.Execute(s.Ctx, "label", "ns", "random", "watch=this")
	s.Assertions.Contains(out, "namespace/random labeled")
	s.NoError(err)
}

func (s *testingSuite) unwatchNamespace() {
	// Label the `random` namespace with the watchNamespaceSelector labels
	// kubectl label ns random watch-
	out, _, err := s.TestHelper.Execute(s.Ctx, "label", "ns", "random", "watch-")
	s.Assertions.Contains(out, "namespace/random unlabeled")
	s.NoError(err)
}

func (s *testingSuite) addLabelToSecondNamespace() {
	// label the `random` namespace
	// kubectl label ns random inconsequential=label
	out, _, err := s.TestHelper.Execute(s.Ctx, "label", "ns", "random", "inconsequential=label")
	s.Assertions.Contains(out, "namespace/random labeled")
	s.NoError(err)
}

func (s *testingSuite) removeLabelFromSecondNamespace() {
	// unlabel the `random` namespace
	// kubectl label ns random inconsequential-
	out, _, err := s.TestHelper.Execute(s.Ctx, "label", "ns", "random", "inconsequential-")
	s.Assertions.Contains(out, "namespace/random unlabeled")
	s.NoError(err)
}

func (s *testingSuite) applyFile(filename string, extraArgs ...string) {
	err := s.TestHelper.ApplyFile(s.Ctx, filename, extraArgs...)
	s.NoError(err)
}

func (s *testingSuite) deleteFile(filename string, extraArgs ...string) {
	err := s.TestHelper.DeleteFile(s.Ctx, filename, extraArgs...)
	s.NoError(err)
}
