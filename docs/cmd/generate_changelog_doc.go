package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/google/go-github/v31/github"
	"github.com/rotisserie/eris"
	"github.com/solo-io/go-utils/versionutils"
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
			return generateMinorReleaseChangelog(args)
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

func generateMinorReleaseChangelog(args []string) error {
	if len(args) != 1 {
		return InvalidInputError(fmt.Sprintf("%v", len(args)-1))
	}
	target := args[0]
	var (
		err error
	)
	switch target {
	case glooDocGen:
		err = generateGlooChangelog()
	case glooEDocGen:
		err = generateGlooEChangelog()
	default:
		return InvalidInputError(target)
	}

	return err
}

func generateGlooChangelog() error {
	client := github.NewClient(nil)
	allReleases, err := getAllReleases(client, "gloo")
	if err != nil {
		return err
	}

	err = parseReleases(allReleases)
	if err != nil {
		return err
	}
	return nil
}

func generateGlooEChangelog() error {
	// Initialize Auth
	ctx := context.Background()
	if os.Getenv("GITHUB_TOKEN") == "" {
		return MissingGithubTokenError()
	}
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	// Get all Gloo OSS release changelogs
	allGlooEReleases, err := getAllReleases(client, "solo-projects")
	if err != nil {
		return err
	}

	err = parseGlooEReleases(allGlooEReleases)
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

func parseGlooEReleases(allGlooEReleases []*github.RepositoryRelease) error {
	var minorReleaseMap = make(map[Version]string)
	for _, release := range allGlooEReleases {
		var releaseTag = release.GetTagName()
		version, err := versionutils.ParseVersion(releaseTag)
		if err != nil {
			return err
		}
		minorVersion := Version{
			Major: version.Major,
			Minor: version.Minor,
		}
		depVersion, err := getGlooDependencyForGlooEVersion(version.String())
		var glooOSSDescription string
		if err == nil {
			// Intended output:  {{enterprise version}} (Uses Gloo OSS [v1.6.x](...)
			glooOssLink := strings.ReplaceAll(depVersion.String(), ".", "")
			glooOSSDescription = fmt.Sprintf("(Uses Gloo OSS [%s](/reference/changelog/open_source/#%s))", depVersion.String(), glooOssLink)
		}
		minorReleaseMap[minorVersion] = minorReleaseMap[minorVersion] + fmt.Sprintf("##### %s %s\n ", version.String(), glooOSSDescription) + release.GetBody()
	}

	var versions Versions
	for minorVersion, _ := range minorReleaseMap {
		versions = append(versions, minorVersion)
	}
	sort.Sort(versions)
	for _, version := range versions {
		body := minorReleaseMap[version]
		fmt.Printf("### v%v.%v\n\n", version.Major, version.Minor)
		fmt.Printf("%v", body)
	}
	return nil
}

func getGlooDependencyForGlooEVersion(versionTag string) (*versionutils.Version, error) {
	dependencyUrl := fmt.Sprintf("https://storage.googleapis.com/gloo-ee-dependencies/%s/dependencies", versionTag[1:])
	request, err := http.NewRequest("GET", dependencyUrl, nil)
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	re, err := regexp.Compile(`.*gloo.*(v.*)`)
	if err != nil {
		return nil, err
	}
	matches := re.FindStringSubmatch(string(body))
	if len(matches) != 2 {
		return nil, eris.Errorf("unable to get gloo dependency for gloo enterprise version %s\n response from google storage API: %s", versionTag, string(body))
	}
	glooVersionTag := matches[1]
	version, err := versionutils.ParseVersion(glooVersionTag)
	if err != nil {
		return nil, err
	}
	return version, nil
}

func parseReleases(releases []*github.RepositoryRelease) error {
	var minorReleaseMap = make(map[Version]string)
	for _, release := range releases {
		var releaseTag = release.GetTagName()
		version, err := versionutils.ParseVersion(releaseTag)
		if err != nil {
			return err
		}
		minorVersion := Version{
			Major: version.Major,
			Minor: version.Minor,
		}
		minorReleaseMap[minorVersion] = minorReleaseMap[minorVersion] + fmt.Sprintf("##### %v\n", version.String()) + release.GetBody()
	}

	var versions Versions
	for minorVersion, _ := range minorReleaseMap {
		versions = append(versions, minorVersion)
	}
	sort.Sort(versions)
	for _, version := range versions {
		body := minorReleaseMap[version]
		fmt.Printf("### v%v.%v\n\n", version.Major, version.Minor)
		fmt.Printf("%v", body)
	}
	return nil
}

type Version versionutils.Version
type Versions []Version

// The following functions are used to display the releases in order of release version
func (v Version) LessThan(version Version) bool {
	result, _ := versionutils.Version(v).IsGreaterThanOrEqualTo(versionutils.Version(version))
	return result
}

func (s Versions) Len() int {
	return len(s)
}

func (s Versions) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s Versions) Less(i, j int) bool {
	return s[i].LessThan(s[j])
}
