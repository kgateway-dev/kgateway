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
	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/suite"
	"golang.org/x/oauth2"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/cmdutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/threadsafe"
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

// fetchGithubRelease pages through releases ordered by creation date descending
// (newest first, skipping drafts) using the GitHub GraphQL API, which
// guarantees the ordering via orderBy. Returns the tag name of the first release for
// which match returns true.
func fetchGithubRelease(ctx context.Context, opts ReleaseOptions, match func(tagName string) (bool, error)) (string, error) {
	opts.applyDefaults()
	client := newGraphQLClient()

	var query struct {
		Repository struct {
			Releases struct {
				Nodes []struct {
					TagName githubv4.String
					IsDraft githubv4.Boolean
				}
				PageInfo struct {
					EndCursor   githubv4.String
					HasNextPage githubv4.Boolean
				}
			} `graphql:"releases(first: 100, after: $cursor, orderBy: {field: CREATED_AT, direction: DESC})"`
		} `graphql:"repository(owner: $owner, name: $repo)"`
	}

	variables := map[string]any{
		"cursor": (*githubv4.String)(nil),
		"owner":  githubv4.String(opts.Org),
		"repo":   githubv4.String(opts.Repo),
	}

	for {
		if err := client.Query(ctx, &query, variables); err != nil {
			return "", fmt.Errorf("graphql query releases: %w", err)
		}
		for _, r := range query.Repository.Releases.Nodes {
			if bool(r.IsDraft) || string(r.TagName) == "" {
				continue
			}
			ok, err := match(string(r.TagName))
			if err != nil {
				return "", err
			}
			if ok {
				return string(r.TagName), nil
			}
		}
		if !bool(query.Repository.Releases.PageInfo.HasNextPage) {
			break
		}
		variables["cursor"] = new(query.Repository.Releases.PageInfo.EndCursor)
	}
	return "", fmt.Errorf("no matching release found")
}

// FetchLatestRelease returns the most recent release tag that is an ancestor of HEAD.
// This mirrors `git describe --tags --abbrev=0` but works in shallow checkouts where
// tags are not fetched, by resolving HEAD via git then checking ancestry via the GitHub API.
func FetchLatestRelease(ctx context.Context, opts ReleaseOptions) (string, error) {
	opts.applyDefaults()

	var shaOut threadsafe.Buffer
	if err := cmdutils.Command(ctx, "git", "rev-parse", "HEAD").
		WithStdout(&shaOut).
		WithStderr(&shaOut).
		Run(); err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	headSHA := strings.TrimSpace(shaOut.String())

	// Use REST client for CompareCommits — no GraphQL equivalent.
	restClient := github.NewClient(nil)
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		restClient = github.NewClient(nil).WithAuthToken(token)
	}

	return fetchGithubRelease(ctx, opts, func(tagName string) (bool, error) {
		// Compare tag...HEAD: status=="ahead" means HEAD is ahead of the tag (tag is an ancestor).
		comparison, _, err := restClient.Repositories.CompareCommits(ctx, opts.Org, opts.Repo, tagName, headSHA, nil)
		if err != nil {
			return false, fmt.Errorf("compare %s...%s: %w", tagName, headSHA, err)
		}
		return comparison.GetStatus() == "ahead" || comparison.GetStatus() == "identical", nil
	})
}

func FetchPreviousMinorRelease(ctx context.Context, latestRelease string, opts ReleaseOptions) (string, error) {
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
