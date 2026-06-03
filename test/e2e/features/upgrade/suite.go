//go:build e2e

package upgrade

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/go-github/v67/github"
	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/cmdutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils/kubectl"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/threadsafe"
	"github.com/kgateway-dev/kgateway/v2/pkg/version"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/helpers"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

var (
	_                        e2e.NewSuiteFunc = NewTestingSuite
	setupManifest                             = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	GatewayNameLabelSelector                  = "gateway.networking.k8s.io/gateway-name=gateway"
)

// testingSuite validates that kgateway can be upgraded from a released version to the locally-built chart.
// The parent test function (TestUpgrade) is responsible for:
//   - Installing kgateway from the remote release before this suite runs.
//   - Uninstalling kgateway after this suite completes.
type testingSuite struct {
	*base.BaseTestingSuite
	podSelectors []string
}

func NewTestingSuiteWithConfig(ctx context.Context, testInst *e2e.TestInstallation, podSelectors []string) func(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return func(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
		return &testingSuite{
			BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, base.TestCase{}, nil),
			podSelectors:     podSelectors,
		}
	}
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, base.TestCase{}, nil),
		// This should be updated to app.kubernetes.io/component=proxy. v2.2.x did not have this label
		podSelectors: []string{GatewayNameLabelSelector},
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
	s.TestInstallation.AssertionsT(s.T()).EventuallyKgatewayUpgradeSucceeded(s.Ctx, version.Version)
	s.assertNoKgatewayPodErrors()

	// Ensure the proxy pod is also updated
	for _, selector := range s.podSelectors {
		s.TestInstallation.AssertionsT(s.T()).EventuallyPodHasImageVersion(s.Ctx, "default", selector, version.Version)
	}

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

// assertNoKgatewayPodErrors fetches logs from all kgateway pods and fails if any error-level log lines are found.
func (s *testingSuite) assertNoKgatewayPodErrors() {
	ns := s.TestInstallation.Metadata.InstallNamespace
	pods, err := s.TestInstallation.Actions.Kubectl().GetPodsInNsWithLabel(s.Ctx, ns, defaults.KGatewayPodLabel)
	s.Require().NoError(err, "failed to list kgateway pods in namespace %s", ns)
	s.Require().NotEmpty(pods, "no kgateway pods found in namespace %s", ns)

	for _, pod := range pods {
		logs, err := s.TestInstallation.Actions.Kubectl().GetContainerLogs(s.Ctx, ns, pod,
			kubectl.WithContainer(helpers.KgatewayContainerName))
		s.Require().NoError(err, "failed to get logs for pod %s", pod)

		for i, line := range strings.Split(logs, "\n") {
			lower := strings.ToLower(line)
			s.Assert().False(
				strings.Contains(lower, `"level":"error"`) || strings.Contains(lower, `"level": "error"`),
				"error log found in pod %s line %d: %s", pod, i+1, line,
			)
		}
	}
}

func newRestClient() *github.Client {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return github.NewClient(nil)
	}
	return github.NewClient(nil).WithAuthToken(token)
}

// FetchReleaseOptions configures the GitHub owner and repo used by FetchLatestRelease
// and FetchPreviousMinorRelease. Zero values default to "kgateway-dev"/"kgateway".
type FetchReleaseOptions struct {
	Owner string
	Repo  string
}

func (o FetchReleaseOptions) owner() string {
	if o.Owner != "" {
		return o.Owner
	}
	return "kgateway-dev"
}

func (o FetchReleaseOptions) repo() string {
	if o.Repo != "" {
		return o.Repo
	}
	return "kgateway"
}

// fetchGithubRelease pages through releases ordered by creation date descending
// (newest first, skipping drafts) using the GitHub REST API. Returns the tag name of the
// first release for which match returns true.
func fetchGithubRelease(ctx context.Context, opts FetchReleaseOptions, match func(tagName string) (bool, error)) (string, error) {
	client := newRestClient()
	listOpts := &github.ListOptions{PerPage: 100}

	for {
		releases, resp, err := client.Repositories.ListReleases(ctx, opts.owner(), opts.repo(), listOpts)
		if err != nil {
			return "", fmt.Errorf("list releases page %d: %w", listOpts.Page, err)
		}
		for _, r := range releases {
			if r.GetDraft() || r.GetTagName() == "" {
				continue
			}
			ok, err := match(r.GetTagName())
			if err != nil {
				return "", err
			}
			if ok {
				return r.GetTagName(), nil
			}
		}
		if resp.NextPage == 0 {
			break
		}
		listOpts.Page = resp.NextPage
	}
	return "", fmt.Errorf("no matching release found")
}

// FetchLatestRelease returns the most recent release tag that is an ancestor of HEAD.
// This mirrors `git describe --tags --abbrev=0` but works in shallow checkouts where
// tags are not fetched, by resolving HEAD via git then checking ancestry via the GitHub API.
func FetchLatestRelease(ctx context.Context, opts FetchReleaseOptions) (string, error) {
	var shaOut threadsafe.Buffer
	if err := cmdutils.Command(ctx, "git", "rev-parse", "HEAD").
		WithStdout(&shaOut).
		WithStderr(&shaOut).
		Run(); err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	headSHA := strings.TrimSpace(shaOut.String())

	restClient := newRestClient()

	return fetchGithubRelease(ctx, opts, func(tagName string) (bool, error) {
		// Compare tag...HEAD: status=="ahead" means HEAD is ahead of the tag (tag is an ancestor).
		comparison, _, err := restClient.Repositories.CompareCommits(ctx, opts.owner(), opts.repo(), tagName, headSHA, nil)
		if err != nil {
			return false, fmt.Errorf("compare %s...%s: %w", tagName, headSHA, err)
		}
		return comparison.GetStatus() == "ahead" || comparison.GetStatus() == "identical", nil
	})
}

func FetchPreviousMinorRelease(ctx context.Context, latestRelease string, opts FetchReleaseOptions) (string, error) {
	parts := strings.Split(latestRelease, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("unexpected tag format: %s", latestRelease)
	}
	minorInt, convErr := strconv.Atoi(parts[1])
	if convErr != nil {
		return "", fmt.Errorf("failed to parse minor version from tag %q: %v", latestRelease, convErr)
	}
	if minorInt <= 0 {
		return "", fmt.Errorf("no previous minor for release %q (minor is 0)", latestRelease)
	}
	previousMinorPrefix := fmt.Sprintf("%s.%d.", parts[0], minorInt-1)
	return fetchGithubRelease(ctx, opts, func(tagName string) (bool, error) {
		return strings.HasPrefix(tagName, previousMinorPrefix), nil
	})
}
