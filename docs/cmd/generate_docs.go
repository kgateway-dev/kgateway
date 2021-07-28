package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/solo-io/go-utils/securityscanutils"

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
		log.Fatalf("unable to run: %v\n", err)
	}
}

type options struct {
	ctx        context.Context
	targetRepo string
}

func rootApp(ctx context.Context) *cobra.Command {
	opts := &options{
		ctx: ctx,
	}
	app := &cobra.Command{}
	app.AddCommand(changelogMdFromGithubCmd(opts))
	app.AddCommand(securityScanMdFromCmd(opts))
	app.AddCommand(enterpriseHelmValuesMdFromGithubCmd(opts))
	app.AddCommand(getReleasesCmd(opts))
	app.AddCommand(runSecurityScanCmd(opts))
	app.PersistentFlags().StringVarP(&opts.targetRepo, "TargetRepo", "r", glooOpenSourceRepo, "specify one of 'gloo' or 'glooe'")
	_ = app.MarkFlagRequired("TargetRepo")

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

// Pulls scan results from google cloud bucket during docs generation.
// Then generates a human-readable single page for all our security scan results.
func securityScanMdFromCmd(opts *options) *cobra.Command {
	app := &cobra.Command{
		Use:   "gen-security-scan-md",
		Short: "pull down security scan files from gcloud bucket and generate docs markdown file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if os.Getenv(skipSecurityScan) != "" {
				return nil
			}
			return generateSecurityScanMd(opts)
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
			return generateChangelogMd(opts)
		},
	}
	return app
}

func enterpriseHelmValuesMdFromGithubCmd(opts *options) *cobra.Command {
	app := &cobra.Command{
		Use:   "get-enterprise-helm-values",
		Short: "Get documentation of valid helm values from Gloo Enterprise github",
		RunE: func(cmd *cobra.Command, args []string) error {
			if os.Getenv(skipEnterpriseDocsGeneration) != "" {
				return nil
			}
			return fetchEnterpriseHelmValues(args)
		},
	}
	return app
}

// Command for running the actual security scan on the images
// running this runs trivy on all images for versions greater than
// MIN_SCANNED_VERSION.
// Uploads scanning results to github security tab and google cloud bucket.
func runSecurityScanCmd(opts *options) *cobra.Command {

	app := &cobra.Command{
		Use:   "run-security-scan",
		Short: "runs trivy scans on images from repo specified",
		Long:  "runs trivy vulnerability scans on images from the repo specified. Only reports HIGH and CRITICAL-level vulnerabilities and uploads scan results to google cloud bucket and github security page",
		RunE: func(cmd *cobra.Command, args []string) error {
			err := scanGlooImages(opts.ctx)
			return err
		},
	}
	return app
}

// Fetches releases and serializes them and prints to stdout.
// This is meant to be used so that releases can be cached locally for multiple tasks
// such as security scanning, changelog generation
// rather than fetch all repo releases per task and risk hitting GitHub ratelimit
func fetchAndSerializeReleases(opts *options) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if !useCachedReleases() {
			return nil
		}
		client, err := githubutils.GetClient(opts.ctx)
		if err != nil {
			return err
		}
		switch opts.targetRepo {
		case glooDocGen:
			err = getRepoReleases(opts.ctx, glooOpenSourceRepo, client)
		case glooEDocGen:
			err = getRepoReleases(opts.ctx, glooEnterpriseRepo, client)
		default:
			return InvalidInputError(opts.targetRepo)
		}
		return err
	}
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
	glooDocGen                   = "gloo"
	glooEDocGen                  = "glooe"
	skipChangelogGeneration      = "SKIP_CHANGELOG_GENERATION"
	skipSecurityScan             = "SKIP_SECURITY_SCAN"
	skipEnterpriseDocsGeneration = "SKIP_ENTERPRISE_DOCS_GENERATION"
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
	FileNotFoundError = func(path string, branch string) error {
		return eris.Errorf("Could not find file at path %s on branch %s", path, branch)
	}
)

// Generates changelog for releases as fetched from Github
// Github defaults to a chronological order
func generateChangelogMd(opts *options) error {
	client := githubutils.GetClientOrExit(context.Background())
	switch opts.targetRepo {
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
		return InvalidInputError(opts.targetRepo)
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
func generateSecurityScanMd(opts *options) error {
	var err error
	switch opts.targetRepo {
	case glooDocGen:
		err = generateSecurityScanGloo(context.Background())
	case glooEDocGen:
		err = generateSecurityScanGlooE(context.Background())
	default:
		return InvalidInputError(opts.targetRepo)
	}

	return err
}

func scanGlooImages(ctx context.Context) error {
	var (
		stableOnlyConstraint *semver.Constraints
		err                  error
	)
	minVersionToScan := os.Getenv("MIN_SCANNED_VERSION")
	if minVersionToScan == "" {
		log.Println("MIN_SCANNED_VERSION environment variable not set, scanning all versions from repo")
	} else {
		stableOnlyConstraint, err = semver.NewConstraint(fmt.Sprintf(">= %s", minVersionToScan))
		if err != nil {
			log.Fatalf("Invalid constraint version: %s", minVersionToScan)
		}
	}
	scanner := &securityscanutils.SecurityScanner{
		Repos: []*securityscanutils.SecurityScanRepo{
			{
				Repo:  "gloo",
				Owner: "solo-io",
				Opts: &securityscanutils.SecurityScanOpts{
					OutputDir: "_output/scans",
					ImagesPerVersion: map[string][]string{
						">=v1.6.0": OpenSourceImages(),
					},
					VersionConstraint:      stableOnlyConstraint,
					ImageRepo:              "quay.io/solo-io",
					UploadCodeScanToGithub: true,
				},
			},
			{
				Repo:  "solo-projects",
				Owner: "solo-io",
				Opts: &securityscanutils.SecurityScanOpts{
					OutputDir: "_output/scans",
					ImagesPerVersion: map[string][]string{
						"<=1.6.x":  EnterpriseImages(true),
						">=v1.7.x": EnterpriseImages(false),
					},
					VersionConstraint:           stableOnlyConstraint,
					ImageRepo:                   "quay.io/solo-io",
					UploadCodeScanToGithub:      false,
					CreateGithubIssuePerVersion: true,
				},
			},
		},
	}
	return scanner.GenerateSecurityScans(ctx)
}

func generateSecurityScanGloo(ctx context.Context) error {
	// Initialize Auth
	client, err := githubutils.GetClient(ctx)
	if err != nil {
		return err
	}
	var allReleases []*github.RepositoryRelease
	if useCachedReleases() {
		allReleases = getCachedReleases(glooCachedReleasesFile)
	} else {
		allReleases, err = githubutils.GetAllRepoReleases(ctx, client, "solo-io", glooOpenSourceRepo)
		if err != nil {
			return err
		}
	}
	githubutils.SortReleasesBySemver(allReleases)
	versionsToScan := getVersionsToScan(allReleases)
	return BuildSecurityScanReportGloo(versionsToScan)
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
	versionsToScan := getVersionsToScan(allReleases)
	return BuildSecurityScanReportGlooE(versionsToScan)
}

func fetchEnterpriseHelmValues(args []string) error {
	ctx := context.Background()
	client, err := githubutils.GetClient(ctx)
	if err != nil {
		return err
	}

	// Download the file at the specified path on the latest released branch of solo-projects
	path := "install/helm/gloo-ee/reference/values.txt"
	semverReleaseTag, err := ioutil.ReadFile("../_output/gloo-enterprise-version")
	if err != nil {
		return err
	}
	version, err := semver.NewVersion(string(semverReleaseTag))
	if err != nil {
		return err
	}
	minorReleaseTag := fmt.Sprintf("v%d.%d.x", version.Major(), version.Minor())
	files, err := githubutils.GetFilesFromGit(ctx, client, "solo-io", glooEnterpriseRepo, minorReleaseTag, path)
	if err != nil {
		return err
	}
	if len(files) <= 0 {
		return FileNotFoundError(path, minorReleaseTag)
	}

	// Decode the file and log to the console
	decodedContents, err := base64.StdEncoding.DecodeString(*files[0].Content)
	if err != nil {
		return err
	}
	fmt.Printf("%s", decodedContents)

	return nil
}

func getVersionsToScan(releases []*github.RepositoryRelease) []string {
	var (
		versions             []string
		stableOnlyConstraint *semver.Constraints
		err                  error
	)
	minVersionToScan := os.Getenv("MIN_SCANNED_VERSION")
	if minVersionToScan == "" {
		log.Println("MIN_SCANNED_VERSION environment variable not set, scanning all versions from repo")
	} else {
		stableOnlyConstraint, err = semver.NewConstraint(fmt.Sprintf(">= %s", minVersionToScan))
		if err != nil {
			log.Fatalf("Invalid constraint version: %s", minVersionToScan)
		}
	}

	for _, release := range releases {
		// ignore beta releases when display security scan results
		test, err := semver.NewVersion(release.GetTagName())
		if err != nil {
			continue
		}
		if stableOnlyConstraint == nil || stableOnlyConstraint.Check(test) {
			versions = append(versions, test.String())
		}
	}
	return versions
}
