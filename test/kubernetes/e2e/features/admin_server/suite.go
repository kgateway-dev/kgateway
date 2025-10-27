//go:build e2e

package admin_server

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/test/controllerutils/admincli"
	"github.com/kgateway-dev/kgateway/v2/test/helpers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

var (
	// manifests
	gatewayWithRouteManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway-with-route.yaml")

	// objects
	kgatewayDeploymentObjectMeta = metav1.ObjectMeta{
		Name:      helpers.DefaultKgatewayDeploymentName,
		Namespace: "kgateway-test",
	}

	// test cases
	setup = base.TestCase{
		Manifests: []string{
			defaults.HttpbinManifest,
			gatewayWithRouteManifest,
		},
	}
	testCases = map[string]*base.TestCase{
		// no test-specific manifests
	}
)

type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

func (s *testingSuite) TestXdsSnapshot() {
	s.TestInstallation.Assertions.AssertKgatewayAdminApi(
		s.Ctx,
		kgatewayDeploymentObjectMeta,
		s.xdsSnapshotAssertion(),
	)
}

func (s *testingSuite) TestKrtSnapshot() {
	s.TestInstallation.Assertions.AssertKgatewayAdminApi(
		s.Ctx,
		kgatewayDeploymentObjectMeta,
		s.krtSnapshotAssertion(),
	)
}

func (s *testingSuite) TestPprof() {
	s.TestInstallation.Assertions.AssertKgatewayAdminApi(
		s.Ctx,
		kgatewayDeploymentObjectMeta,
		s.pprofAssertion(),
	)
}

func (s *testingSuite) TestLogging() {
	s.TestInstallation.Assertions.AssertKgatewayAdminApi(
		s.Ctx,
		kgatewayDeploymentObjectMeta,
		s.loggingAssertion(),
	)
}

func (s *testingSuite) TestVersion() {
	s.TestInstallation.Assertions.AssertKgatewayAdminApi(
		s.Ctx,
		kgatewayDeploymentObjectMeta,
		s.versionAssertion(),
	)
}

func (s *testingSuite) xdsSnapshotAssertion() func(ctx context.Context, adminClient *admincli.Client) {
	return func(ctx context.Context, adminClient *admincli.Client) {
		s.TestInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
			xdsSnapshot, err := adminClient.GetXdsSnapshot(ctx)
			g.Expect(err).NotTo(gomega.HaveOccurred(), "can get xds snapshot")
			g.Expect(xdsSnapshot).NotTo(gomega.BeEmpty(), "xds snapshot is not empty")
		}).
			WithContext(ctx).
			WithTimeout(time.Second * 10).
			WithPolling(time.Millisecond * 200).
			Should(gomega.Succeed())
	}
}

func (s *testingSuite) krtSnapshotAssertion() func(ctx context.Context, adminClient *admincli.Client) {
	return func(ctx context.Context, adminClient *admincli.Client) {
		s.TestInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
			krtSnapshot, err := adminClient.GetKrtSnapshot(ctx)
			g.Expect(err).NotTo(gomega.HaveOccurred(), "can get krt snapshot")
			g.Expect(krtSnapshot).NotTo(gomega.BeEmpty(), "krt snapshot is not empty")
		}).
			WithContext(ctx).
			WithTimeout(time.Second * 10).
			WithPolling(time.Millisecond * 200).
			Should(gomega.Succeed())
	}
}

func (s *testingSuite) pprofAssertion() func(ctx context.Context, adminClient *admincli.Client) {
	return func(ctx context.Context, adminClient *admincli.Client) {
		s.TestInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
			pprofResponse, err := adminClient.GetPprof(ctx, "goroutine")
			g.Expect(err).NotTo(gomega.HaveOccurred(), "can get pprof goroutines response")
			g.Expect(pprofResponse).NotTo(gomega.BeEmpty(), "pprof goroutines response is not empty")
		}).
			WithContext(ctx).
			WithTimeout(time.Second * 10).
			WithPolling(time.Millisecond * 200).
			Should(gomega.Succeed())
	}
}

func (s *testingSuite) loggingAssertion() func(ctx context.Context, adminClient *admincli.Client) {
	return func(ctx context.Context, adminClient *admincli.Client) {
		s.TestInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
			loggingResponse, err := adminClient.GetLogging(ctx)
			g.Expect(err).NotTo(gomega.HaveOccurred(), "can get logging response")
			g.Expect(loggingResponse).NotTo(gomega.BeEmpty(), "logging response is not empty")
		}).
			WithContext(ctx).
			WithTimeout(time.Second * 10).
			WithPolling(time.Millisecond * 200).
			Should(gomega.Succeed())
	}
}

func (s *testingSuite) versionAssertion() func(ctx context.Context, adminClient *admincli.Client) {
	return func(ctx context.Context, adminClient *admincli.Client) {
		s.TestInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
			resp, err := adminClient.GetVersion(ctx)
			g.Expect(err).NotTo(gomega.HaveOccurred(), "can get version response")
			g.Expect(resp["version"]).NotTo(gomega.BeEmpty(), "version field present")
			g.Expect(resp["string"]).To(gomega.ContainSubstring("controller version"), "string field contains version info")

			// Ensure the version field matches the version embedded in the string
			full := resp["string"]
			const prefix = "controller version "
			start := strings.Index(full, prefix)
			g.Expect(start).ToNot(gomega.Equal(-1), "string contains controller version prefix")
			remainder := full[start+len(prefix):]
			end := strings.Index(remainder, ",")
			g.Expect(end).ToNot(gomega.Equal(-1), "string contains a comma after version")
			embeddedVersion := remainder[:end]
			g.Expect(embeddedVersion).To(gomega.Equal(resp["version"]), "embedded version matches version field")
		}).
			WithContext(ctx).
			WithTimeout(time.Second * 10).
			WithPolling(time.Millisecond * 200).
			Should(gomega.Succeed())
	}
}
