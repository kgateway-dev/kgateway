package github_action_utils

import (
	"context"
	"math"
	"os"

	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/go-utils/changelogutils"
	"github.com/solo-io/go-utils/versionutils"
	"github.com/solo-io/go-utils/vfsutils"
)

// GetVersion computes an appropriate semver string based on the contents of
// the ./changelog directory fount at repoRootPath
func GetVersion(repoRootPath string, repo string, owner string) (string, error) {
	ctx := context.Background()

	// compute largest local changelog version
	version, err := getLargestLocalChangelogVersion(ctx, repoRootPath, owner, repo, changelogutils.ChangelogDirectory)
	if err != nil {
		return "", err
	}

	PR_NUMBER := os.Getenv(PR_NUMBER)
	if len(PR_NUMBER) == 0 { // implies we are computing a release's version
		return version.String(), nil
	} else { // implies we are computing a PR's version
		return version.String() + "-pr" + PR_NUMBER, nil
	}
}

// GetLatestEnterpriseVersion computes the latest Gloo Enterprise version.
// It is intended to be used by the Makefile.docs, ostensibly as a variable for
// filling out correctly-referenced enterprise docs
func GetLatestEnterpriseVersion(repoRootPath string, repo string, owner string) error {
	ctx := context.Background()

	localVersion, err := getLargestLocalChangelogVersion(ctx, repoRootPath, owner, repo, changelogutils.ChangelogDirectory)
	if err != nil {
		return err
	}
	// only version constraints we care about come from Gloo major/minor version
	maxGlooEVersion := &versionutils.Version{
		Major: localVersion.Major,
		Minor: localVersion.Minor,
		Patch: math.MaxInt32,
	}

	os.Mkdir("./_output", 0755)
	f, err := os.Create("./_output/gloo-enterprise-version")
	if err != nil {
		return err
	}
	defer f.Close()
	enterpriseVersion, err := version.GetLatestHelmChartVersionWithMaxVersion(version.EnterpriseHelmRepoIndex, version.GlooEE, true, maxGlooEVersion)
	if err != nil {
		return err
	}
	f.WriteString(enterpriseVersion)
	f.Sync()

	return nil
}

func getLargestLocalChangelogVersion(ctx context.Context, repoRootPath, owner, repo, changelogDirPath string) (*versionutils.Version, error) {
	mountedRepo, err := vfsutils.NewLocalMountedRepoForFs(repoRootPath, owner, repo)
	if err != nil {
		return nil, changelogutils.MountLocalDirectoryError(err)
	}
	files, err := mountedRepo.ListFiles(ctx, changelogDirPath)
	if err != nil {
		return nil, changelogutils.ReadChangelogDirError(err)
	}
	zero := versionutils.Zero()
	largestVersion := &zero
	for _, file := range files {
		if file.IsDir() {
			curVersion, err := versionutils.ParseVersion(file.Name())
			if err != nil {
				continue
			}
			if curVersion.MustIsGreaterThan(*largestVersion) {
				largestVersion = curVersion
			}
		}
	}

	if largestVersion == &zero {
		return nil, errors.Errorf("unable to find any versions at repo root %v with changelog dir %v", repoRootPath, changelogDirPath)
	}

	return largestVersion, nil
}
