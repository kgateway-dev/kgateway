package version

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/solo-io/go-utils/changelogutils"
	"github.com/solo-io/go-utils/versionutils"
	"github.com/solo-io/skv2/codegen/util"
)

// GetLastReleaseOfCurrentBranch returns the Version of the latest patch version for the current minor version
// To avoid querying the Github API, we use the changelog folder to determine this version
// Be aware, that while this has the benefit of not using an external API,
// we may hit issues where a release failed and therefore the returned version is not actually published.
func GetLastReleaseOfCurrentBranch() (*versionutils.Version, error) {
	changelogDir := filepath.Join(util.GetModuleRoot(), "changelog")
	directoryEntries, err := os.ReadDir(changelogDir)
	if err != nil {
		return nil, changelogutils.ReadChangelogDirError(err)
	}

	var latestVersions []*versionutils.Version

	for _, dirEntry := range directoryEntries {
		if !dirEntry.IsDir() {
			// We can ignore validation.yaml and any other files
			continue
		}

		changelogVersion, parseErr := versionutils.ParseVersion(dirEntry.Name())
		if parseErr != nil {
			// This would happen if a changelog is poorly formatted
			// We don't want to let that break this functionality
			continue
		}

		latestVersions = append(latestVersions, changelogVersion)
		sort.Slice(latestVersions, func(i, j int) bool {
			var version1 = *latestVersions[i]
			var version2 = *latestVersions[j]
			return !version2.MustIsGreaterThanOrEqualTo(version1)
		})

		// We only care about the 2 latest versions
		if len(latestVersions) > 2 {
			latestVersions = latestVersions[:2]
		}
	}

	// The latest version will be for a changelog entry that is in progress, but not yet released.
	// Therefore, we always take the second latest
	secondLatestChangelogVersion := latestVersions[1]
	return secondLatestChangelogVersion, nil
}
