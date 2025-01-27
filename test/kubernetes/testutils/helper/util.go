package helper

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/google/go-github/v32/github"
	. "github.com/onsi/gomega"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/test/testutils"
	"github.com/solo-io/gloo/test/testutils/version"
	"github.com/solo-io/go-utils/changelogutils"
	"github.com/solo-io/go-utils/githubutils"
	"github.com/solo-io/go-utils/versionutils"
	"github.com/solo-io/skv2/codegen/util"
)

// Deprecated; if this is needed create a resource yaml for it.
func GetHttpEchoImage() string {
	httpEchoImage := "hashicorp/http-echo"
	if runtime.GOARCH == "arm64" {
		httpEchoImage = "gcr.io/solo-test-236622/http-echo:0.2.4"
	}
	return httpEchoImage
}

// For nightly runs, we want to install a released version rather than using a locally built chart
// To do this, set the environment variable RELEASED_VERSION with either a version name or "LATEST" to get the last release
func GetTestReleasedVersion(ctx context.Context, repoName string) string {
	releasedVersion := os.Getenv(testutils.ReleasedVersion)

	if releasedVersion == "" {
		// In the case where the released version is empty, we return an empty string
		// The function which consumes this value will then use the locally built chart
		return releasedVersion
	}

	if releasedVersion == "LATEST" {
		_, current, err := GetUpgradeVersions(ctx, repoName)
		Expect(err).NotTo(HaveOccurred())
		return current.String()
	}

	// Assume that releasedVersion is a valid version, for a previously released version of Gloo Edge
	return releasedVersion
}

func GetTestHelperForRootDir(ctx context.Context, rootDir, namespace string) (*SoloTestHelper, error) {
	if useVersion := GetTestReleasedVersion(ctx, "gloo"); useVersion != "" {
		return NewSoloTestHelper(func(defaults TestConfig) TestConfig {
			defaults.RootDir = rootDir
			defaults.HelmChartName = "gloo"
			defaults.InstallNamespace = namespace
			defaults.ReleasedVersion = useVersion
			defaults.Verbose = true
			return defaults
		})
	} else {
		return NewSoloTestHelper(func(defaults TestConfig) TestConfig {
			defaults.RootDir = rootDir
			defaults.HelmChartName = "gloo"
			defaults.InstallNamespace = namespace
			defaults.Verbose = true
			return defaults
		})
	}
}

// GetUpgradeVersions returns two semantic versions of a repository:
//   - prevLtsRelease: the latest patch release of v1.m-1.x
//   - latestRelease:  the latest patch release of v1.m.x
//
// Originally intended for use in upgrade testing, it can return any of:
//   - (prevLtsRelease, latestRelease, nil): all release versions computable
//   - (prevLtsRelease, nil, nil):           only prevLtsRelease computable (ie current branch has never been released)
//   - (nil, nil, err):                      unable to fetch versions for upgrade test
func GetUpgradeVersions(ctx context.Context, repoName string) (*versionutils.Version, *versionutils.Version, error) {
	// get the latest and upcoming releases of the current branch
	files, changelogReadErr := os.ReadDir(filepath.Join(util.GetModuleRoot(), changelogutils.ChangelogDirectory))
	if changelogReadErr != nil {
		return nil, nil, changelogutils.ReadChangelogDirError(changelogReadErr)
	}
	latestRelease, upcomingRelease, upcomingReleaseErr := version.ChangelogDirForLatestRelease(files...)
	if upcomingReleaseErr != nil && !errors.Is(upcomingReleaseErr, version.FirstReleaseError) {
		return nil, nil, upcomingReleaseErr
	}

	// get latest release of previous LTS branch
	// TODO(nfuden): Update goutils to not use a struct but rather interface so we can test this more easily.
	client, githubClientErr := githubutils.GetClient(ctx)
	if githubClientErr != nil {
		return nil, nil, errors.Wrapf(githubClientErr, "unable to create github client")
	}
	prevLtsRelease, prevLtsReleaseErr := getLatestReleasedPatchVersion(ctx, client, repoName, upcomingRelease.Major, upcomingRelease.Minor-1)
	if prevLtsReleaseErr != nil {
		return nil, nil, prevLtsReleaseErr
	}

	if upcomingReleaseErr != nil {
		// if we don't yet have a release for the current branch, we can only upgrade from prevLtsRelease
		return prevLtsRelease, nil, nil
	} else {
		// otherwise, we can upgrade from both prevLtsRelease -and- latestRelease
		return prevLtsRelease, latestRelease, nil
	}
}

type latestPatchForMinorPredicate struct {
	versionPrefix string
}

func (s *latestPatchForMinorPredicate) Apply(release *github.RepositoryRelease) bool {
	return strings.HasPrefix(*release.Name, s.versionPrefix) &&
		!release.GetPrerelease() && // we don't want a prerelease version
		!strings.Contains(release.GetBody(), "This release build failed") && // we don't want a failed build
		release.GetPublishedAt().Before(time.Now().In(time.UTC).Add(time.Duration(-60)*time.Minute))
}

func newLatestPatchForMinorPredicate(versionPrefix string) *latestPatchForMinorPredicate {
	return &latestPatchForMinorPredicate{
		versionPrefix: versionPrefix,
	}
}

// getLatestReleasedPatchVersion will return the latest released patch version for the given major and minor version
// NOTE: this attempts to reach out to github to get the latest release
func getLatestReleasedPatchVersion(ctx context.Context, client *github.Client, repoName string, majorVersion, minorVersion int) (*versionutils.Version, error) {

	versionPrefix := fmt.Sprintf("v%d.%d", majorVersion, minorVersion)
	releases, err := githubutils.GetRepoReleasesWithPredicateAndMax(ctx, client, "solo-io", repoName, newLatestPatchForMinorPredicate(versionPrefix), 1)
	if err != nil {
		return nil, errors.Wrap(err, "unable to get releases")
	}
	if len(releases) == 0 {
		return nil, errors.Errorf("Could not find a recent release with version prefix: %s", versionPrefix)
	}
	v, err := versionutils.ParseVersion(*releases[0].Name)
	if err != nil {
		return nil, errors.Wrapf(err, "error parsing release name")
	}
	return v, nil
}
