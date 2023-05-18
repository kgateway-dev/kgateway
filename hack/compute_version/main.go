package main

import (
	"context"
	"fmt"
	"os"

	"github.com/solo-io/gloo/hack/helpers"
	"github.com/solo-io/go-utils/changelogutils"
)

const (
	repoRootPath = "."
	repo         = "gloo"
	owner        = "solo-io"
)

func main() {
	ctx := context.Background()

	// compute largest local changelog version
	version, err := helpers.GetLargestLocalChangelogVersion(ctx, repoRootPath, owner, repo, changelogutils.ChangelogDirectory)
	if err != nil {
		panic(err)
	}

	PR_NUMBER := os.Getenv("PR_NUMBER")
	if len(PR_NUMBER) == 0 {
		// if there is no PR_NUMBER, we are computing a release's version
		fmt.Print(version.String())
	} else {
		// if there *is* a PR_NUBMER, we are computing a PR's version
		fmt.Print(version.String() + "-pr" + PR_NUMBER)
	}
}
