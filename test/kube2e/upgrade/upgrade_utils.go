package upgrade

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/google/go-github/v32/github"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/test/testutils/version"
	"github.com/solo-io/go-utils/changelogutils"
	"github.com/solo-io/go-utils/githubutils"
	"github.com/solo-io/go-utils/versionutils"
)

// GetUpgradeVersions for the given repo.
// This will return the lastminor, currentminor, and an error
// This may return lastminor + currentminor, or just lastminor and an error or a just an error
func GetUpgradeVersions(ctx context.Context, repoName string) (lastMinorLatestPatchVersion *versionutils.Version, currentMinorLatestPatchVersion *versionutils.Version, err error) {

	twoBack, currentBranch, curMinorErr := getLastReleaseOfCurrentMinor()
	currentMinorLatestPatchVersion = twoBack
	if errors.Is(curMinorErr, version.FirstReleaseError) {
		// we are on the first release of a minor
		// we should use this branch rather than the last release
		currentMinorLatestPatchVersion = currentBranch
	} else if curMinorErr != nil {
		return nil, nil, curMinorErr
	}

	// TODO(nfuden): Update goutils to not use a struct but rather interface
	// so we can test this more easily.
	client, err := githubutils.GetClient(ctx)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "unable to create github client")
	}

	var currentMinorLatestRelease *versionutils.Version
	// we dont believe there should be a minor release yet so its ok to not do this extra computation
	if curMinorErr == nil {
		var currentMinorLatestReleaseError error
		// we may get a changelog value that does not have a github release - get the latest release for current minor
		currentMinorLatestRelease, currentMinorLatestReleaseError = getLatestReleasedPatchVersion(ctx, client, repoName, currentMinorLatestPatchVersion.Major, currentMinorLatestPatchVersion.Minor)
		if currentMinorLatestReleaseError != nil {
			return nil, lastMinorLatestPatchVersion, currentMinorLatestReleaseError
		}
	}

	lastMinorLatestPatchVersion, lastMinorErr := getLatestReleasedPatchVersion(ctx, client, repoName, currentMinorLatestPatchVersion.Major, currentMinorLatestPatchVersion.Minor-1)
	if lastMinorErr != nil {
		// a true error lets return that.
		return nil, nil, lastMinorErr
	}

	// last minor should never be nil, currentMinor and curMinorerr MAY be nil
	return lastMinorLatestPatchVersion, currentMinorLatestRelease, curMinorErr
}

func getLastReleaseOfCurrentMinor() (*versionutils.Version, *versionutils.Version, error) {
	// pull out to const
	_, filename, _, _ := runtime.Caller(0) //get info about what is calling the function
	fParts := strings.Split(filename, string(os.PathSeparator))
	splitIdx := 0
	// In all cases the home of the project will be one level above test - this handles forks as well as the standard case /home/runner/work/gloo/gloo/test/kube2e/upgrade/junit.xml
	for idx, dir := range fParts {
		if dir == "test" {
			splitIdx = idx - 1
		}
	}
	pathToChangelogs := filepath.Join(fParts[:splitIdx+1]...)
	pathToChangelogs = filepath.Join(pathToChangelogs, changelogutils.ChangelogDirectory)
	pathToChangelogs = string(os.PathSeparator) + pathToChangelogs

	files, err := os.ReadDir(pathToChangelogs)
	if err != nil {
		return nil, nil, changelogutils.ReadChangelogDirError(err)
	}

	return version.ChangelogDirForLatestRelease(files...)
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
