//go:build e2e

package upgrade

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/cmdutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

// proxyNamespace and proxyLabelSelector identify the data-plane proxy that the controller
// provisions for the Gateway defined in testdata/setup.yaml.
const (
	proxyNamespace     = "default"
	proxyLabelSelector = "gateway.networking.k8s.io/gateway-name=gateway"
)

var (
	_             e2e.NewSuiteFunc = NewTestingSuite
	setupManifest                  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	version       string
)

func init() {
	// Default to the version used in CI
	version = envutils.GetOrDefault("VERSION", "v1.0.0-ci1", false)
}

// testingSuite validates that kgateway can be upgraded from a released version to the locally-built chart.
// The parent test function (TestUpgrade) is responsible for:
//   - Installing kgateway from the remote release before this suite runs.
//   - Uninstalling kgateway after this suite completes.
type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, base.TestCase{}, nil),
	}
}

func (s *testingSuite) SetupSuite() {
	s.BaseTestingSuite.SetupSuite()
	// kgateway was installed from a released version by the parent test function.
	// Verify it is healthy before attempting the upgrade.
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayInstallSucceeded(s.Ctx)
}

func (s *testingSuite) applyManifests() func() {
	s.ApplyManifests(&base.TestCase{
		Manifests: []string{setupManifest, defaults.HttpbinManifest},
	})

	return func() {
		s.DeleteManifests(&base.TestCase{
			Manifests: []string{setupManifest, defaults.HttpbinManifest},
		})
	}
}

// TestUpgrade upgrades both the CRD chart and the controller chart from the previously installed
// remote release to the locally-built chart, then verifies the installation is healthy.
func (s *testingSuite) TestUpgrade() {
	// Create a gateway and ensure it works as expected
	cleanup := s.applyManifests()
	testutils.Cleanup(s.T(), cleanup)

	s.T().Logf("checking connectivity with the gateway...")
	common.SetupBaseGateway(s.Ctx, s.T(), s.TestInstallation, types.NamespacedName{
		Name:      "gateway",
		Namespace: "default",
	})
	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{StatusCode: http.StatusOK},
		curl.WithPath("/"),
		curl.WithHostHeader("example.com"),
		curl.WithPort(8080),
	)
	s.T().Logf(" ok")

	s.TestInstallation.InstallKgatewayCRDsFromLocalChart(s.Ctx, s.T())
	s.TestInstallation.InstallKgatewayCoreFromLocalChart(s.Ctx, s.T())

	// Verify kgateway control plane upgraded successfully.
	s.T().Logf("checking the kgateway deployment && pod...")
	s.TestInstallation.AssertionsT(s.T()).EventuallyKgatewayUpgradeSucceeded(s.Ctx, version)
	s.T().Logf(" ok")

	// Ensure the proxy data plane was upgraded too: the Deployment must finish rolling out
	// (old-revision proxy pods fully scaled down) and every proxy pod must run the new image
	s.T().Logf("checking the proxy deployment...")
	s.TestInstallation.AssertionsT(s.T()).EventuallyDeploymentsRolledOut(s.Ctx, proxyNamespace, proxyLabelSelector)
	s.T().Logf(" ok")
	s.T().Logf("checking the proxy image tag...")
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsHaveImageVersion(s.Ctx, proxyNamespace, proxyLabelSelector, version)
	s.T().Logf(" ok")

	// Ensure the same gateway works after the upgrade.
	s.T().Logf("checking connectivity with the gateway after the upgrade...")
	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{StatusCode: http.StatusOK},
		curl.WithPath("/"),
		curl.WithHostHeader("example.com"),
		curl.WithPort(8080),
	)
	s.T().Logf(" ok")

	// Recreate the same gateway and ensure it works after the upgrade
	cleanup()
	s.applyManifests()
	s.T().Logf("checking connectivity with the gateway after recreating it...")
	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{StatusCode: http.StatusOK},
		curl.WithPath("/"),
		curl.WithHostHeader("example.com"),
		curl.WithPort(8080),
	)
	s.T().Logf(" ok")
}

// FetchLatestRelease returns the most recent release tag that is an ancestor of HEAD.
// This mirrors `git describe --tags --abbrev=0` but works in shallow checkouts where
// tags are not fetched, by resolving HEAD via git then checking ancestry via the GitHub API.
func FetchLatestRelease(ctx context.Context) (string, error) {
	script := filepath.Join(fsutils.GetModuleRoot(), "hack", "get-release.sh")
	var stdout bytes.Buffer
	cmd := cmdutils.Command(ctx, script, "--latest").
		WithStdout(&stdout).
		WithStderr(os.Stderr)
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

// FetchLatestRelease returns the most recent n-1 release tag that is an ancestor of HEAD.
// This mirrors `git describe --tags --abbrev=0` but works in shallow checkouts where
// tags are not fetched, by resolving HEAD via git then checking ancestry via the GitHub API.
func FetchPreviousMinorRelease(ctx context.Context) (string, error) {
	script := filepath.Join(fsutils.GetModuleRoot(), "hack", "get-release.sh")
	var stdout bytes.Buffer
	cmd := cmdutils.Command(ctx, script, "--previous").
		WithStdout(&stdout).
		WithStderr(os.Stderr)
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}
