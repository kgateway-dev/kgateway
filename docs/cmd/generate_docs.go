package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"log"
	"os"

	"github.com/spf13/pflag"

	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/util/version"

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
func runSecurityScanCmd(opts *options) *cobra.Command {
	scanOptions := &runSecurityScanOptions{
		options: opts,
	}

	app := &cobra.Command{
		Use:   "run-security-scan",
		Short: "runs trivy scans on images from repo specified",
		Long:  "runs trivy vulnerability scans on images from the repo specified. Only reports HIGH and CRITICAL-level vulnerabilities and uploads scan results to google cloud bucket and github security page",
		RunE: func(cmd *cobra.Command, args []string) error {
			err := scanImagesForRepo(scanOptions.ctx, scanOptions.targetRepo, scanOptions.vulnerabilityAction)
			return err
		},
	}

	scanOptions.addToFlags(app.Flags())

	return app
}

type runSecurityScanOptions struct {
	*options

	vulnerabilityAction string
}

func (r *runSecurityScanOptions) addToFlags(flags *pflag.FlagSet) {
	flags.StringVarP(&r.vulnerabilityAction, "vulnerability-action", "a", "none", "action to take when a vulnerability is discovered {none, github-issue-all, github-issue-latest}")
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
	repoOwner          = "solo-io"
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
		MainRepo:              glooEnterpriseRepo,
		DependentRepo:         glooOpenSourceRepo,
		RepoOwner:             repoOwner,
		MainRepoReleases:      getCachedReleases(glooeCachedReleasesFile),
		DependentRepoReleases: getCachedReleases(glooCachedReleasesFile),
	}
	depFn, err := changelogdocutils.GetOSDependencyFunc(repoOwner, glooEnterpriseRepo, glooOpenSourceRepo, ghToken)
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
	bArray, err := os.ReadFile(fileName)
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

// scanImagesForRepo executes a SecurityScan for the repo provided
func scanImagesForRepo(ctx context.Context, targetRepo string, vulnerabilityAction string) error {
	contextutils.SetLogLevel(zapcore.DebugLevel)
	logger := contextutils.LoggerFrom(ctx)

	versionConstraint, err := getScannerVersionConstraint()
	if err != nil {
		logger.Fatalf("Invalid constraint version: %v", err)
	}
	if versionConstraint == nil {
		// to be extra-safe, we should require devs to configure a constraint
		logger.Fatalf("No version constraint defined")
	}

	outputDir := getScannerOutputDir()
	logger.Debugf("Scanner will write results to directory: %s", outputDir)

	var securityScanRepos []*securityscanutils.SecurityScanRepo

	if targetRepo == glooDocGen {
		securityScanRepos = append(securityScanRepos, &securityscanutils.SecurityScanRepo{
			Repo:  glooOpenSourceRepo,
			Owner: repoOwner,
			Opts: &securityscanutils.SecurityScanOpts{
				OutputDir: outputDir,
				ImagesPerVersion: map[string][]string{
					">=v1.12.3":            OpenSourceImages(version.MustParseSemantic("1.12.3")),  //extra images: [kubectl]
					"<v1.12.3, >=v1.12.0":  OpenSourceImages(version.MustParseSemantic("1.12.0")),  //extra images: []
					"<v1.12.0, >=v1.11.28": OpenSourceImages(version.MustParseSemantic("1.11.28")), //extra images: [gateway, kubectl]
					"<v1.11.28, >=v1.10.0": OpenSourceImages(version.MustParseSemantic("1.10.0")),  //extra images: [gateway]
				},
				VersionConstraint:                      versionConstraint,
				ImageRepo:                              "quay.io/solo-io",
				UploadCodeScanToGithub:                 false,
				CreateGithubIssuePerVersion:            vulnerabilityAction == "github-issue-all",
				CreateGithubIssueForLatestPatchVersion: vulnerabilityAction == "github-issue-latest",
			},
		})
	}

	if targetRepo == glooEDocGen {
		securityScanRepos = append(securityScanRepos, &securityscanutils.SecurityScanRepo{
			Repo:  glooEnterpriseRepo,
			Owner: repoOwner,
			Opts: &securityscanutils.SecurityScanOpts{
				OutputDir: outputDir,
				ImagesPerVersion: map[string][]string{
					">=v1.7.x": EnterpriseImages(version.MustParseSemantic("1.7.0")),
				},
				VersionConstraint:                      versionConstraint,
				ImageRepo:                              "quay.io/solo-io",
				UploadCodeScanToGithub:                 false,
				CreateGithubIssuePerVersion:            vulnerabilityAction == "github-issue-all",
				CreateGithubIssueForLatestPatchVersion: vulnerabilityAction == "github-issue-latest",
			},
		})
	}

	if securityScanRepos == nil {
		logger.Fatalf("No repositories were targeted to be scanned")
	}

	scanner := &securityscanutils.SecurityScanner{
		Repos: securityScanRepos,
	}
	return scanner.GenerateSecurityScans(ctx)
}

func getScannerVersionConstraint() (*semver.Constraints, error) {
	if versionConstraint := os.Getenv("VERSION_CONSTRAINT"); versionConstraint != "" {
		return semver.NewConstraint(fmt.Sprintf("%s", versionConstraint))
	}

	if minVersionToScan := os.Getenv("MIN_SCANNED_VERSION"); minVersionToScan != "" {
		return semver.NewConstraint(fmt.Sprintf(">= %s", minVersionToScan))
	}

	// no constraint applied
	return nil, nil
}

// getScannerOutputDir returns the local directory where scans will be accumulated
func getScannerOutputDir() string {
	if scanDir := os.Getenv("SCAN_DIR"); scanDir != "" {
		return scanDir
	}
	return "_output/scans"
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
		allReleases, err = githubutils.GetAllRepoReleases(ctx, client, repoOwner, glooOpenSourceRepo)
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
		allReleases, err = githubutils.GetAllRepoReleases(ctx, client, repoOwner, glooEnterpriseRepo)
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
	semverReleaseTag, err := os.ReadFile("../_output/gloo-enterprise-version")
	if err != nil {
		return err
	}
	version, err := semver.NewVersion(string(semverReleaseTag))
	if err != nil {
		return err
	}
	minorReleaseTag := fmt.Sprintf("v%d.%d.x", version.Major(), version.Minor())
	files, err := githubutils.GetFilesFromGit(ctx, client, repoOwner, glooEnterpriseRepo, minorReleaseTag, path)
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
