package main

import (
	"context"
	"fmt"
	"os"
	"regexp"

	"github.com/google/go-github/v31/github"
	"github.com/rotisserie/eris"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2"
)

func main() {
	ctx := context.Background()
	app := rootApp(ctx)
	if err := app.Execute(); err != nil {
		fmt.Printf("unable to run: %v\n", err)
		os.Exit(1)
	}
}

type options struct {
	ctx              context.Context
	HugoDataSoloOpts HugoDataSoloOpts
}

type HugoDataSoloOpts struct {
	product string
	version string
	// if set, will override the version when rendering the
	callLatest bool
	noScope    bool
}

func rootApp(ctx context.Context) *cobra.Command {
	opts := &options{
		ctx: ctx,
	}
	app := &cobra.Command{
		Use: "docs-util",
		RunE: func(cmd *cobra.Command, args []string) error {

			return nil
		},
	}
	app.AddCommand(changelogMdFromGithubCmd(opts))
	app.AddCommand(minorReleaseChangelogMdFromGithubCmd(opts))

	app.PersistentFlags().StringVar(&opts.HugoDataSoloOpts.version, "version", "", "version of docs and code")
	app.PersistentFlags().StringVar(&opts.HugoDataSoloOpts.product, "product", "gloo", "product to which the docs refer (defaults to gloo)")
	app.PersistentFlags().BoolVar(&opts.HugoDataSoloOpts.noScope, "no-scope", false, "if set, will not nest the served docs by product or version")
	app.PersistentFlags().BoolVar(&opts.HugoDataSoloOpts.callLatest, "call-latest", false, "if set, will use the string 'latest' in the scope, rather than the particular release version")

	return app
}

func changelogMdFromGithubCmd(opts *options) *cobra.Command {
	app := &cobra.Command{
		Use:   "gen-changelog-md",
		Short: "generate a markdown file from Github Release pages API",
		RunE: func(cmd *cobra.Command, args []string) error {
			if os.Getenv(skipChangelogGeneration) != "" {
				return nil
			}
			return generateChangelogMd(args)
		},
	}
	return app
}

func minorReleaseChangelogMdFromGithubCmd(opts *options) *cobra.Command {
	app := &cobra.Command{
		Use:   "gen-minor-releases-changelog-md",
		Short: "generate an aggregated changelog markdown file for each minor release version",
		RunE: func(cmd *cobra.Command, args []string) error {
			if os.Getenv(skipChangelogGeneration) != "" {
				return nil
			}
			return generateMinorReleaseChangelog()
		},
	}
	return app
}

const (
	latestVersionPath = "latest"
)

const (
	glooDocGen              = "gloo"
	glooEDocGen             = "glooe"
	skipChangelogGeneration = "SKIP_CHANGELOG_GENERATION"
)

var (
	InvalidInputError = func(arg string) error {
		return eris.Errorf("invalid input, must provide exactly one argument, either '%v' or '%v', (provided %v)",
			glooDocGen,
			glooEDocGen,
			arg)
	}
	MissingGithubTokenError = func() error {
		return eris.Errorf("Must either set GITHUB_TOKEN or set %s environment variable to true", skipChangelogGeneration)
	}
)

func generateChangelogMd(args []string) error {
	if len(args) != 1 {
		return InvalidInputError(fmt.Sprintf("%v", len(args)-1))
	}
	client := github.NewClient(nil)
	target := args[0]
	var repo string
	switch target {
	case glooDocGen:
		repo = "gloo"
	case glooEDocGen:
		repo = "solo-projects"
		ctx := context.Background()
		if os.Getenv("GITHUB_TOKEN") == "" {
			return MissingGithubTokenError()
		}
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
		)
		tc := oauth2.NewClient(ctx, ts)
		client = github.NewClient(tc)
	default:
		return InvalidInputError(target)
	}

	allReleases, err := getAllReleases(client, repo)
	if err != nil {
		return err
	}

	for _, release := range allReleases {
		fmt.Printf("### %v\n\n", *release.TagName)
		fmt.Printf("%v", *release.Body)
	}
	return nil
}

func generateMinorReleaseChangelog() error {
	client := github.NewClient(nil)
	// get changelogs from gloo releases
	repo := "gloo"
	allReleases, err := getAllReleases(client, repo)
	if err != nil {
		return err
	}

	var releaseList []*github.RepositoryRelease
	for _, release := range allReleases {
		releaseList = append(releaseList, release)
	}

	err = parseReleases(releaseList)
	if err != nil {
		return err
	}
	return nil
}

func getAllReleases(client *github.Client, repo string) ([]*github.RepositoryRelease, error) {
	allReleases, _, err := client.Repositories.ListReleases(context.Background(), "solo-io", repo,
		&github.ListOptions{
			Page:    0,
			PerPage: 10000000,
		})
	if err != nil {
		return nil, err
	}
	return allReleases, nil
}

func parseReleases(releases []*github.RepositoryRelease) error {
	var minorReleaseMap = make(map[string]string)
	for _, release := range releases {
		var releaseTag = release.GetTagName()
		version, err := parseVersionTag(releaseTag)
		if err != nil {
			return err
		}
		minorVersion := fmt.Sprintf("v%s.%s", version.MajorRelease, version.MinorRelease)
		minorReleaseMap[minorVersion] = minorReleaseMap[minorVersion] + fmt.Sprintf("##### %v\n", version.Tag) + release.GetBody()
	}

	for minorVersion, body := range minorReleaseMap {
		fmt.Printf("### %v\n\n", minorVersion)
		fmt.Printf("%v", body)
	}
	return nil
}

func parseVersionTag(versionTag string) (*Version, error) {
	var version *Version
	versionRegexp := regexp.MustCompile("^v([0-9]+)\\.([0-9]+)\\.([0-9]+)(?:-([0-9a-zA-Z-]+(?:\\.[0-9a-zA-Z-]+)*))?(?:\\+([0-9a-zA-Z-]+(?:\\.[0-9a-zA-Z-]+)*))?$")
	matches := versionRegexp.FindStringSubmatch(versionTag)
	if len(matches) < 5 {
		return nil, fmt.Errorf("tag %s is not formatted correctly, %+v", versionTag, matches)
	}
	version = &Version{
		Tag:               matches[0],
		MajorRelease:      matches[1],
		MinorRelease:      matches[2],
		Patch:             matches[3],
		PreReleaseVersion: matches[4],
	}
	return version, nil
}

type Release struct {
	Version         *Version
	Fixes           string
	DependencyBumps string
	NewFeatures     string
	HelmChanges     string
	BreakingChanges string
	CVEs            string
}

type Version struct {
	Tag               string
	MajorRelease      string
	MinorRelease      string
	Patch             string
	PreReleaseVersion string
}
