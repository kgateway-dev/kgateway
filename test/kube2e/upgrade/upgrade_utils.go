package upgrade

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/google/go-github/v32/github"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/go-utils/changelogutils"
	"github.com/solo-io/go-utils/githubutils"
	"github.com/solo-io/go-utils/versionutils"
)

var (
	FirstReleaseError = "First Release of Minor"
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

func GetUpgradeVersions(ctx context.Context, repoName string) (lastMinorLatestPatchVersion *versionutils.Version, currentMinorLatestPatchVersion *versionutils.Version, err error) {
	currentMinorLatestPatchVersion, curMinorErr := getLastReleaseOfCurrentMinor(repoName)
	if curMinorErr != nil {
		if curMinorErr.Error() != FirstReleaseError {
			return nil, nil, curMinorErr
		}
	}
	lastMinorLatestPatchVersion, lastMinorErr := getLatestReleasedVersion(ctx, currentMinorLatestPatchVersion.Major, currentMinorLatestPatchVersion.Minor-1)
	if lastMinorErr != nil {
		return nil, nil, lastMinorErr
	}
	return lastMinorLatestPatchVersion, currentMinorLatestPatchVersion, curMinorErr
}

func getLastReleaseOfCurrentMinor(repoName string) (*versionutils.Version, error) {
	// pull out to const
	_, filename, _, _ := runtime.Caller(0) //get info about what is calling the function
	fmt.Printf(filename)
	fParts := strings.Split(filename, string(os.PathSeparator))
	splitIdx := 0
	//we can end up in a situation where the path contains the repo_name twice when running in ci - keep going until we find the last use ex: /home/runner/work/gloo/gloo/test/kube2e/upgrade/junit.xml
	for idx, dir := range fParts {
		if dir == repoName {
			splitIdx = idx
		}
	}
	pathToChangelogs := filepath.Join(fParts[:splitIdx+1]...)
	pathToChangelogs = filepath.Join(pathToChangelogs, changelogutils.ChangelogDirectory)
	pathToChangelogs = string(os.PathSeparator) + pathToChangelogs

	files, err := os.ReadDir(pathToChangelogs)
	if err != nil {
		return nil, changelogutils.ReadChangelogDirError(err)
	}

	versions := make([]*versionutils.Version, len(files)-1) //ignore validation file
	for idx, f := range files {
		if f.Name() != "validation.yaml" {
			version, err := versionutils.ParseVersion(f.Name())
			if err != nil {
				return nil, errors.Errorf("Could not get version for changelog folder: %s\n", f.Name())
			}
			versions[idx] = version
		}
	}

	sort.Sort(ByVersion(versions))
	//first release of minor
	if versions[len(versions)-1].Minor != versions[len(versions)-2].Minor {
		return versions[len(versions)-1], errors.Errorf(FirstReleaseError)
	}
	return versions[len(versions)-2], nil
}

func getLatestReleasedVersion(ctx context.Context, majorVersion, minorVersion int) (*versionutils.Version, error) {
	client, err := githubutils.GetClient(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to create github client")
	}
	versionPrefix := fmt.Sprintf("v%d.%d", majorVersion, minorVersion)

	// inexact version requested may be prerelease and not have assets
	// We do assume that within a minor version we use monotonically increasing patch numbers
	// We also assume that the first release that is not strict semver is technically the largest
	for i := 0; i < 5; i++ {
		// Get the next page of
		listOpts := github.ListOptions{Page: i, PerPage: 10} // max per request
		releases, _, err := client.Repositories.ListReleases(ctx, "solo-io", "gloo", &listOpts)
		if err != nil {
			return nil, errors.Wrapf(err, "error listing releases")
		}

		for _, release := range releases {
			v, err := versionutils.ParseVersion(*release.Name)
			if err != nil {
				continue
			}

			// either a major-minor was specified something of the form v%d.%d
			// or are searching for latest stable and have found the most recent
			// experimental and are now searching for a conforming release
			if versionPrefix != "" {
				// take the first valid from this version
				// as we assume increasing ordering
				if strings.HasPrefix(v.String(), versionPrefix) {
					return v, nil
				}
				continue
			}
		}
	}

	return nil, errors.Errorf("Could not find a recent release with version prefix: %s", versionPrefix)
}
