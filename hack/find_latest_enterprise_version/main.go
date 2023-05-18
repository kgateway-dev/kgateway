package main

import (
	"context"
	"log"
	"math"
	"os"

	"github.com/solo-io/gloo/hack/helpers"
	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/go-utils/changelogutils"
	"github.com/solo-io/go-utils/versionutils"
)

func main() {
	ctx := context.Background()
	repoRootPath := "."
	owner := "solo-io"
	repo := "gloo"
	changelogDirPath := changelogutils.ChangelogDirectory

	localVersion, err := helpers.GetLargestLocalChangelogVersion(ctx, repoRootPath, owner, repo, changelogDirPath)
	if err != nil {
		log.Fatal(err)
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
		log.Fatal(err)
	}
	defer f.Close()
	enterpriseVersion, err := version.GetLatestHelmChartVersionWithMaxVersion(version.EnterpriseHelmRepoIndex, version.GlooEE, true, maxGlooEVersion)
	if err != nil {
		log.Fatal(err)
	}
	f.WriteString(enterpriseVersion)
	f.Sync()
}
