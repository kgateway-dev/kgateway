//go:build e2e

package upgrade

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/suite"
	"golang.org/x/oauth2"
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

	s.TestInstallation.InstallKgatewayCRDsFromLocalChart(s.Ctx, s.T())
	s.TestInstallation.InstallKgatewayCoreFromLocalChart(s.Ctx, s.T())
	s.TestInstallation.AssertionsT(s.T()).EventuallyKgatewayUpgradeSucceeded(s.Ctx, version)

	// Ensure the proxy data plane was upgraded too: the Deployment must finish rolling out
	// (old-revision proxy pods fully scaled down) and every proxy pod must run the new image
	s.TestInstallation.AssertionsT(s.T()).EventuallyDeploymentsRolledOut(s.Ctx, proxyNamespace, proxyLabelSelector)
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsHaveImageVersion(s.Ctx, proxyNamespace, proxyLabelSelector, version)

	// Ensure the same gateway works after the upgrade
	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{StatusCode: http.StatusOK},
		curl.WithPath("/"),
		curl.WithHostHeader("example.com"),
		curl.WithPort(8080),
	)

	// Recreate the same gateway and ensure it works after the upgrade
	cleanup()
	s.applyManifests()
	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{StatusCode: http.StatusOK},
		curl.WithPath("/"),
		curl.WithHostHeader("example.com"),
		curl.WithPort(8080),
	)
}

func newGraphQLClient() *githubv4.Client {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return githubv4.NewClient(http.DefaultClient)
	}
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	return githubv4.NewClient(oauth2.NewClient(context.Background(), ts))
}

const (
	defaultGitHubOrg  = "kgateway-dev"
	defaultGitHubRepo = "kgateway"
)

// ReleaseOptions configures which GitHub repository to query for releases.
type ReleaseOptions struct {
	// Org is the GitHub organization owning the repository. Defaults to "kgateway-dev".
	Org string
	// Repo is the GitHub repository name. Defaults to "kgateway".
	Repo string
}

func (o *ReleaseOptions) applyDefaults() {
	if o.Org == "" {
		o.Org = defaultGitHubOrg
	}
	if o.Repo == "" {
		o.Repo = defaultGitHubRepo
	}
}

// FetchLatestRelease returns the most recent release tag that is an ancestor of HEAD.
// This mirrors `git describe --tags --abbrev=0` but works in shallow checkouts where
// tags are not fetched, by resolving HEAD via git then checking ancestry via the GitHub API.
func FetchLatestRelease(ctx context.Context, opts ReleaseOptions) (string, error) {
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

func FetchPreviousMinorRelease(ctx context.Context, latestRelease string, opts ReleaseOptions) (string, error) {
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
