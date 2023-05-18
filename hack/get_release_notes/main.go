package main

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"github.com/solo-io/gloo/hack/helpers"
	"github.com/solo-io/go-utils/changelogutils"
	"github.com/solo-io/go-utils/vfsutils"
)

// potentially convert these to CLI inputs
const (
	repoRootPath = "."
	repo         = "gloo"
	owner        = "solo-io"
	outfile      = "changelog-body.md"
)

func main() {
	ctx := context.Background()
	w := bytes.NewBuffer([]byte{})

	// compute largest local changelog version
	version, err := helpers.GetLargestLocalChangelogVersion(ctx, repoRootPath, owner, repo, changelogutils.ChangelogDirectory)
	lazyManageErr(err)

	// compute changelog for version
	mountedRepo, err := vfsutils.NewLocalMountedRepoForFs(repoRootPath, owner, repo)
	lazyManageErr(changelogutils.MountLocalDirectoryError(err))

	reader := changelogutils.NewChangelogReader(mountedRepo)
	err = changelogutils.GenerateChangelogForTags(ctx, []string{version.String()}, reader, w)
	lazyManageErr(err)

	// write changelog
	f, err := os.Create(outfile)
	lazyManageErr(err)

	_, err = f.WriteString(w.String())
	lazyManageErr(err)
	f.Close()

	// notify calling process of computed tag
	fmt.Print(version.String())
}

func lazyManageErr(err error) {
	if err != nil {
		panic(err)
	}
}
