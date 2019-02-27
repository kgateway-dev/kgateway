package main

import (
	"context"
	"fmt"
	"github.com/solo-io/go-utils/changelogutils"
	"github.com/solo-io/go-utils/docsutils"
	"github.com/solo-io/go-utils/githubutils"
	"github.com/spf13/afero"
)

func main() {
	client, err := githubutils.GetClient(context.TODO())
	if err != nil {
		panic(err)
	}
	fs := afero.NewOsFs()
	latestTag, err := githubutils.FindLatestReleaseTag(context.TODO(), client, "solo-io", "gloo")
	if err != nil {
		panic(err)
	}
	proposedTag, err := changelogutils.GetProposedTag(fs, latestTag, "")
	if err != nil {
		panic(err)
	}
	changelog, err := changelogutils.ComputeChangelog(fs, latestTag, proposedTag, "")
	if err != nil {
		panic(err)
	}
	markdown := changelogutils.GenerateChangelogMarkdown(changelog)
	fmt.Printf(markdown)

	docsutils.CreateDocsPR("solo-io", "gloo", "v0.8.2", "gloo",
		"docs/v1/github.com/solo-io/gloo",
		"docs/v1/github.com/solo-io/solo-kit",
		"docs/v1/gogoproto",
		"docs/v1/google")
}