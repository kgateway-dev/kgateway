package upgrade

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/google/go-github/v32/github"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/go-utils/changelogutils"
	"github.com/solo-io/go-utils/githubutils"
	"github.com/solo-io/go-utils/versionutils"
)

var (
	FirstReleaseError = errors.New("First Release of Minor")
)

// Type used to sort Versions
type ByVersion []*versionutils.Version

func (a ByVersion) Len() int { return len(a) }
func (a ByVersion) Less(i, j int) bool {
	var version1 = *a[i]
	var version2 = *a[j]
	return version2.MustIsGreaterThan(version1)
}
func (a ByVersion) Swap(i, j int) { a[i], a[j] = a[j], a[i] }

// GetUpgradeVersions for the given repo.
// This will return the lastminor, currentminor, and an error
// This may return lastminor + currentminor, or just lastminor and an error or a just an error
func GetUpgradeVersions(ctx context.Context, repoName string) (lastMinorLatestPatchVersion *versionutils.Version, currentMinorLatestPatchVersion *versionutils.Version, err error) {

	currentMinorLatestPatchVersion, curMinorErr := getLastReleaseOfCurrentMinor()
	if curMinorErr != FirstReleaseError {
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
	if curMinorErr == FirstReleaseError {
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

func getLastReleaseOfCurrentMinor() (*versionutils.Version, error) {
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
		return nil, changelogutils.ReadChangelogDirError(err)
	}

	return filterFilesForLatestRelease(files...)
}

// namedEntry extracts the only thing we really care about for a file entry - the name
type namedEntry interface {
	Name() string
}

// filterFilesForLatestRelease will return the latest release of the current minor
// from a set of file entries that mimick our changelog structure
func filterFilesForLatestRelease[T namedEntry](files ...T) (*versionutils.Version, error) {

	if len(files) < 3 {
		return nil, errors.Errorf("Could not get sufficient versions from files: %v\n", files)
	}

	versions := make([]*versionutils.Version, 0, len(files))
	for _, f := range files {
		// we expect there to be files like "validation.yaml"
		// which are not valid changelogs
		version, err := versionutils.ParseVersion(f.Name())
		if err == nil {
			versions = append(versions, version)
		}
	}
	if len(versions) < 2 {
		return nil, errors.Errorf("Could not get sufficient valid versions from files: %v\n", files)
	}

	sort.Sort(ByVersion(versions))
	//first release of minor
	if versions[len(versions)-1].Minor != versions[len(versions)-2].Minor {
		return versions[len(versions)-1], FirstReleaseError
	}
	return versions[len(versions)-2], nil
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
