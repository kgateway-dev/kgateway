package main

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/Masterminds/semver/v3"
	"github.com/google/go-github/v32/github"
	"github.com/rotisserie/eris"
	. "github.com/solo-io/gloo/docs/cmd/securityscanutils"
	changelogdocutils "github.com/solo-io/go-utils/changeloggenutils"
	"github.com/solo-io/go-utils/githubutils"
	"github.com/spf13/cobra"
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
	ctx context.Context
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
	app.AddCommand(securityScanMdFromCmd(opts))
	app.AddCommand(getReleasesCmd(opts))

	return app
}

// Serializes github repository release and prints serialized releases to stdout
// To be used for caching release data for changelog/security scan docsgen.
func getReleasesCmd(opts *options) *cobra.Command {
	app := &cobra.Command{
		Use:   "gen-releases",
		Short: "cache github releases for gloo edge repository",
		RunE:  fetchAndSerializeReleases(opts),
	}
	return app
}

func fetchAndSerializeReleases(opts *options) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if !useCachedReleases() {
			return nil
		}
		if len(args) != 1 {
			return InvalidInputError(fmt.Sprintf("%v", len(args)-1))
		}
		client, err := githubutils.GetClient(opts.ctx)
		if err != nil {
			return err
		}
		target := args[0]
		switch target {
		case glooDocGen:
			err = getRepoReleases(opts.ctx, glooOpenSourceRepo, client)
		case glooEDocGen:
			err = getRepoReleases(opts.ctx, glooEnterpriseRepo, client)
		default:
			return InvalidInputError(target)
		}
		return err
	}
}

func securityScanMdFromCmd(opts *options) *cobra.Command {
	app := &cobra.Command{
		Use:   "gen-security-scan-md",
		Short: "generate a markdown file from gcloud bucket",
		RunE: func(cmd *cobra.Command, args []string) error {
			if os.Getenv(skipSecurityScan) != "" {
				return nil
			}
			return generateSecurityScanMd(args)
		},
	}
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

// Serialized github RepositoryRelease array to be written to file
func getRepoReleases(ctx context.Context, repo string, client *github.Client) error {
	allReleases, err := githubutils.GetAllRepoReleases(ctx, client, "solo-io", repo)
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err = enc.Encode(allReleases)
	if err != nil {
		return err
	}
	fmt.Print(buf.String())
	return nil
}

const (
	glooDocGen              = "gloo"
	glooEDocGen             = "glooe"
	skipChangelogGeneration = "SKIP_CHANGELOG_GENERATION"
	skipSecurityScan        = "SKIP_SECURITY_SCAN"
)

const (
	glooOpenSourceRepo = "gloo"
	glooEnterpriseRepo = "solo-projects"
)

const (
	glooCachedReleasesFile  = "opensource.out"
	glooeCachedReleasesFile = "enterprise.out"
)

var (
	InvalidInputError = func(arg string) error {
		return eris.Errorf("invalid input, must provide exactly one argument, either '%v' or '%v', (provided %v)",
			glooDocGen,
			glooEDocGen,
			arg)
	}
	MissingGithubTokenError = func(envVar string) error {
		return eris.Errorf("Must either set GITHUB_TOKEN or set %s environment variable to true", envVar)
	}
)

// Generates changelog for releases as fetched from Github
// Github defaults to a chronological order
func generateChangelogMd(args []string) error {
	if len(args) != 1 {
		return InvalidInputError(fmt.Sprintf("%v", len(args)-1))
	}
	client := github.NewClient(nil)
	target := args[0]
	switch target {
	case glooDocGen:
		generator := changelogdocutils.NewMinorReleaseGroupedChangelogGenerator(changelogdocutils.Options{
			MainRepo:         "gloo",
			RepoOwner:        "solo-io",
			MainRepoReleases: getCachedReleases(glooCachedReleasesFile),
		}, client)
		out, err := generator.GenerateJSON(context.Background())
		if err != nil {
			return err
		}
		fmt.Println(out)
	case glooEDocGen:
		err := generateGlooEChangelog()
		if err != nil {
			return err
		}
	default:
		return InvalidInputError(target)
	}

	return nil
}

// Fetches Gloo Enterprise releases, merges in open source release notes, and orders them by version
func generateGlooEChangelog() error {
	// Initialize Auth
	ctx := context.Background()
	ghToken := os.Getenv("GITHUB_TOKEN")
	if ghToken == "" {
		return MissingGithubTokenError(skipChangelogGeneration)
	}
	client, err := githubutils.GetClient(ctx)
	if err != nil {
		return err
	}
	opts := changelogdocutils.Options{
		NumVersions:           200,
		MainRepo:              "solo-projects",
		DependentRepo:         "gloo",
		RepoOwner:             "solo-io",
		MainRepoReleases:      getCachedReleases(glooeCachedReleasesFile),
		DependentRepoReleases: getCachedReleases(glooCachedReleasesFile),
	}
	depFn, err := changelogdocutils.GetOSDependencyFunc("solo-io", "solo-projects", "gloo", ghToken)
	if err != nil {
		return err
	}
	generator := changelogdocutils.NewMergedReleaseGeneratorWithDepFn(opts, client, depFn)
	out, err := generator.GenerateJSON(context.Background())
	if err != nil {
		return err
	}
	fmt.Println(out)
	return nil
}

func getCachedReleases(fileName string) []*github.RepositoryRelease {
	bArray, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil
	}
	buf := bytes.NewBuffer(bArray)
	enc := gob.NewDecoder(buf)
	var releases []*github.RepositoryRelease
	err = enc.Decode(&releases)
	if err != nil {
		return nil
	}
	return releases
}

func useCachedReleases() bool {
	if os.Getenv("USE_CACHED_RELEASES") == "false" {
		return false
	}
	return true
}

// Generates security scan log for releases
func generateSecurityScanMd(args []string) error {
	if len(args) != 1 {
		return InvalidInputError(fmt.Sprintf("%v", len(args)-1))
	}
	target := args[0]
	var (
		err error
	)
	switch target {
	case glooDocGen:
		err = generateSecurityScanGloo(context.Background())
	case glooEDocGen:
		err = generateSecurityScanGlooE(context.Background())
	default:
		return InvalidInputError(target)
	}

	return err
}

func generateSecurityScanGloo(ctx context.Context) error {
	client := github.NewClient(nil)
	var (
		allReleases []*github.RepositoryRelease
		err         error
	)
	if useCachedReleases() {
		allReleases = getCachedReleases(glooCachedReleasesFile)
	} else {
		allReleases, err = githubutils.GetAllRepoReleases(ctx, client, "solo-io", glooOpenSourceRepo)
		if err != nil {
			return err
		}
	}
	githubutils.SortReleasesBySemver(allReleases)

	var tagNames []string
	for _, release := range allReleases {
		// ignore beta releases when display security scan results
		test, err := semver.NewVersion(release.GetTagName())
		stableOnlyConstraint, _ := semver.NewConstraint(">= 1.4.0")
		if err == nil && stableOnlyConstraint.Check(test) {
			tagNames = append(tagNames, release.GetTagName())
		}
	}

	return BuildSecurityScanReportGloo(tagNames)
}

func generateSecurityScanGlooE(ctx context.Context) error {
	// Initialize Auth
	client, err := githubutils.GetClient(ctx)
	if err != nil {
		return err
	}
	var allReleases []*github.RepositoryRelease
	if useCachedReleases() {
		allReleases = getCachedReleases(glooeCachedReleasesFile)
	} else {
		allReleases, err = githubutils.GetAllRepoReleases(ctx, client, "solo-io", glooEnterpriseRepo)
		if err != nil {
			return err
		}
	}
	githubutils.SortReleasesBySemver(allReleases)

	var tagNames []string
	for _, release := range allReleases {
		// ignore beta releases when display security scan results
		test, err := semver.NewVersion(release.GetTagName())
		stableOnlyConstraint, _ := semver.NewConstraint(">= 1.4.0")
		if err == nil && stableOnlyConstraint.Check(test) {
			tagNames = append(tagNames, release.GetTagName())
		}
	}

	return BuildSecurityScanReportGlooE(tagNames)
}
