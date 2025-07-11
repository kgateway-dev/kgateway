//go:build ignore

package version

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/solo-io/go-utils/changelogutils"
	"github.com/solo-io/go-utils/versionutils"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

// GetLastReleaseOfCurrentBranch returns the Version of the latest patch version for the current minor version
// To avoid querying the Github API, we use the changelog folder to determine this version
// Be aware, that while this has the benefit of not using an external API,
// we may hit issues where a release failed and therefore the returned version is not actually published.
func GetLastReleaseOfCurrentBranch() (*versionutils.Version, error) {
	changelogDir := filepath.Join(fsutils.GetModuleRoot(), "changelog")
	directoryEntries, err := os.ReadDir(changelogDir)
	if err != nil {
		return nil, changelogutils.ReadChangelogDirError(err)
	}

	currentlyReleasedVer, _, _ := ChangelogDirForLatestRelease(directoryEntries...)
	return currentlyReleasedVer, nil
}

// namedEntry extracts the only thing we really care about for a file entry - the name
type namedEntry interface {
	Name() string
}

// ChangelogDirForLatestRelease will return the latest release of the current minor
// from a set of file entries that mimick our changelog structure.
// It will also return the currently in flight release and an error
// The error may be FirstReleaseError if the changelog dir is for the first release of a minor version
func ChangelogDirForLatestRelease[T namedEntry](files ...T) (
	currentRelease *versionutils.Version, unreleasedVersion *versionutils.Version, err error) {

	if len(files) < 3 {
		return nil, nil, fmt.Errorf("Could not get sufficient versions from files: %v\n", files)
	}

	versions := make([]*versionutils.Version, 0, len(files))
	for _, f := range files {
		// we expect there to be files like "validation.yaml"
		// which are not valid changelogs
		// there may also be badly formatted entries we should skip
		version, err := versionutils.ParseVersion(f.Name())
		if err == nil {
			versions = append(versions, version)
		}
	}
	if len(versions) < 2 {
		return nil, nil, fmt.Errorf("Could not get sufficient valid versions from files: %v\n", files)
	}

	sort.Sort(sortableVersionSlice(versions))

	if versions[len(versions)-1].Minor != versions[len(versions)-2].Minor {
		err = FirstReleaseError
	}
	return versions[len(versions)-2], versions[len(versions)-1], err
}

var (
	// FirstReleaseError is returned when the changelog dir is for the first release of a minor version
	FirstReleaseError = errors.New("First Release of Minor")
)

// versionSort is a helper type for sorting versions in a slice
type sortableVersionSlice []*versionutils.Version

func (a sortableVersionSlice) Len() int { return len(a) }
func (a sortableVersionSlice) Less(i, j int) bool {
	var version1 = *a[i]
	var version2 = *a[j]
	return version2.MustIsGreaterThan(version1)
}
func (a sortableVersionSlice) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
