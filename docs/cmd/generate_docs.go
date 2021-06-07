package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Masterminds/semver/v3"
	"github.com/google/go-github/v32/github"
	"github.com/rotisserie/eris"
	. "github.com/solo-io/gloo/docs/cmd/securityscanutils"
	changelogdocutils "github.com/solo-io/go-utils/changeloggenutils"
	"github.com/solo-io/go-utils/githubutils"
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
	app.AddCommand(securityScanMdFromCmd(opts))

	app.PersistentFlags().StringVar(&opts.HugoDataSoloOpts.version, "version", "", "version of docs and code")
	app.PersistentFlags().StringVar(&opts.HugoDataSoloOpts.product, "product", "gloo", "product to which the docs refer (defaults to gloo)")
	app.PersistentFlags().BoolVar(&opts.HugoDataSoloOpts.noScope, "no-scope", false, "if set, will not nest the served docs by product or version")
	app.PersistentFlags().BoolVar(&opts.HugoDataSoloOpts.callLatest, "call-latest", false, "if set, will use the string 'latest' in the scope, rather than the particular release version")

	return app
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

const (
	latestVersionPath = "latest"
)

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
			MainRepo:  "gloo",
			RepoOwner: "solo-io",
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
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: ghToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)
	opts := changelogdocutils.Options{
		NumVersions:   200,
		MainRepo:      "solo-projects",
		DependentRepo: "gloo",
		RepoOwner:     "solo-io",
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
	allReleases, err := githubutils.GetAllRepoReleases(ctx, client, "solo-io", glooOpenSourceRepo)
	if err != nil {
		return err
	}
	githubutils.SortReleasesBySemver(allReleases)
	if err != nil {
		return err
	}

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
	if os.Getenv("GITHUB_TOKEN") == "" {
		return MissingGithubTokenError(skipSecurityScan)
	}
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)
	allReleases, err := githubutils.GetAllRepoReleases(ctx, client, "solo-io", glooEnterpriseRepo)
	if err != nil {
		return err
	}
	githubutils.SortReleasesBySemver(allReleases)
	if err != nil {
		return err
	}

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
