//go:build ignore

package watch_namespace_selector

import (
	"context"
	"net/http"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		base.NewBaseTestingSuite(ctx, testInst, setupSuite, testCases),
	}
}

func (s *testingSuite) SetupSuite() {
	s.BaseTestingSuite.SetupSuite()

	// Apply a VS in the install namespace
	s.applyFile(installNamespaceVSManifest, "-n", s.TestInstallation.Metadata.InstallNamespace)
}

func (s *testingSuite) TearDownSuite() {
	// Delete VS in the install namespace
	s.deleteFile(installNamespaceVSManifest, "-n", s.TestInstallation.Metadata.InstallNamespace)

	s.BaseTestingSuite.TearDownSuite()
}

func (s *testingSuite) TestMatchLabels() {
	s.testWatchNamespaceSelector("label", "match")
}

func (s *testingSuite) TestMatchExpressions() {
	s.testWatchNamespaceSelector("expression", "match")
}

func (s *testingSuite) testWatchNamespaceSelector(key, value string) {
	// Ensure the install namespace is watched even if not specified
	s.TestInstallation.Assertions.CurlEventuallyRespondsWithStatus(s.Ctx, "install-ns/", http.StatusOK)

	// Ensure the namespace is not watched
	s.TestInstallation.Actions.Kubectl().Execute(s.Ctx, "label", "ns", "random", key+"-")

	// Ensure CRs defined in non watched-namespaces are not translated
	s.TestInstallation.Assertions.CurlEventuallyRespondsWithStatus(s.Ctx, "random/", http.StatusNotFound)

	s.labelSecondNamespaceAsWatched(key, value)

	// The VS defined in the random namespace should be translated
	s.TestInstallation.Assertions.CurlEventuallyRespondsWithStatus(s.Ctx, "random/", http.StatusOK)

	s.unwatchNamespace(key)

	// Ensure CRs defined in non watched-namespaces are not translated
	s.TestInstallation.Assertions.CurlConsistentlyRespondsWithStatus(s.Ctx, "random/", http.StatusNotFound)
}

func (s *testingSuite) TestUnwatchedNamespaceValidation() {
	s.applyFile(unlabeledRandomNamespaceManifest)
	s.applyFile(randomUpstreamManifest)

	// It should successfully apply inconsequential labels to a ns without validation errors
	s.addInconsequentialLabelToSecondNamespace()
	s.removeInconsequentialLabelFromSecondNamespace()

	// Deleting resources in the namespace and the namespace itself should not error out
	s.deleteFile(randomUpstreamManifest)
	s.deleteFile(unlabeledRandomNamespaceManifest)
}

func (s *testingSuite) TestWatchedNamespaceValidation() {
	s.applyFile(unlabeledRandomNamespaceManifest)
	s.applyFile(randomUpstreamManifest)

	s.labelSecondNamespaceAsWatched("label", "match")

	// It should successfully apply inconsequential labels to a ns we watch without validation errors
	s.addInconsequentialLabelToSecondNamespace()
	s.removeInconsequentialLabelFromSecondNamespace()

	s.Eventually(func() bool {
		err := s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, installNamespaceWithRandomUpstreamVSManifest, "-n", s.TestInstallation.Metadata.InstallNamespace)
		return err == nil
	}, time.Minute*2, time.Second*10)

	// The upstream defined in the random namespace should be translated and referenced
	s.TestInstallation.Assertions.CurlEventuallyRespondsWithStatus(s.Ctx, "/get", http.StatusOK)

	// Trying to unwatch the namespace that has an upstream referenced in another namespace leads to an error
	_, errOut, err := s.TestInstallation.Actions.Kubectl().Execute(s.Ctx, "label", "ns", "random", "label-")

	s.Contains(errOut, `denied the request: resource incompatible with current Gloo snapshot`)
	s.Contains(errOut, `Route Warning: InvalidDestinationWarning. Reason: *v1.Upstream { random.postman-echo } not found`)
	s.Error(err)

	// Trying to delete the namespace also errors out
	_, errOut, err = s.TestInstallation.Actions.Kubectl().Execute(s.Ctx, "delete", "ns", "random")
	s.Contains(errOut, `denied the request: resource incompatible with current Gloo snapshot`)
	s.Contains(errOut, `Route Warning: InvalidDestinationWarning. Reason: *v1.Upstream { random.postman-echo } not found`)
	s.Error(err)

	// Ensure we didn't break the validation server while we're at it
	s.addInconsequentialLabelToSecondNamespace()
	s.removeInconsequentialLabelFromSecondNamespace()

	s.deleteFile(installNamespaceWithRandomUpstreamVSManifest, "-n", s.TestInstallation.Metadata.InstallNamespace)

	s.unwatchNamespace("label")

	// The upstream defined in the random namespace should be translated and referenced
	s.TestInstallation.Assertions.CurlEventuallyRespondsWithStatus(s.Ctx, "/get", http.StatusNotFound)

	// Optimists invent airplanes; pessimists invent parachutes
	s.addInconsequentialLabelToSecondNamespace()
	s.removeInconsequentialLabelFromSecondNamespace()

	s.deleteFile(randomUpstreamManifest)
	s.deleteFile(unlabeledRandomNamespaceManifest)
}

func (s *testingSuite) labelSecondNamespaceAsWatched(key, value string) {
	// Label the `random` namespace with the watchNamespaceSelector labels
	// kubectl label ns random watch=this
	out, _, err := s.TestInstallation.Actions.Kubectl().Execute(s.Ctx, "label", "ns", "random", key+"="+value)
	s.Assertions.Contains(out, "namespace/random labeled")
	s.NoError(err)
}

func (s *testingSuite) unwatchNamespace(key string) {
	// Label the `random` namespace with the watchNamespaceSelector labels
	// kubectl label ns random watch-
	out, _, err := s.TestInstallation.Actions.Kubectl().Execute(s.Ctx, "label", "ns", "random", key+"-")
	s.Assertions.Contains(out, "namespace/random unlabeled")
	s.NoError(err)
}

func (s *testingSuite) addInconsequentialLabelToSecondNamespace() {
	// label the `random` namespace
	// kubectl label ns random inconsequential=label
	out, _, err := s.TestInstallation.Actions.Kubectl().Execute(s.Ctx, "label", "ns", "random", "inconsequential=label")
	s.Assertions.Contains(out, "namespace/random labeled")
	s.NoError(err)
}

func (s *testingSuite) removeInconsequentialLabelFromSecondNamespace() {
	// unlabel the `random` namespace
	// kubectl label ns random inconsequential-
	out, _, err := s.TestInstallation.Actions.Kubectl().Execute(s.Ctx, "label", "ns", "random", "inconsequential-")
	s.Assertions.Contains(out, "namespace/random unlabeled")
	s.NoError(err)
}

func (s *testingSuite) applyFile(filename string, extraArgs ...string) {
	err := s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, filename, extraArgs...)
	s.NoError(err)
}

func (s *testingSuite) deleteFile(filename string, extraArgs ...string) {
	err := s.TestInstallation.Actions.Kubectl().DeleteFile(s.Ctx, filename, extraArgs...)
	s.NoError(err)
}
