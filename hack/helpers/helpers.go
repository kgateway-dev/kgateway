package helpers

import (
	"context"

	"github.com/pkg/errors"
	"github.com/solo-io/go-utils/changelogutils"
	"github.com/solo-io/go-utils/versionutils"
	"github.com/solo-io/go-utils/vfsutils"
)

func GetLargestLocalChangelogVersion(ctx context.Context, repoRootPath, owner, repo, changelogDirPath string) (*versionutils.Version, error) {
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
