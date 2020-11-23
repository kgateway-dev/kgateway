package main

import (
	"context"
	"fmt"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
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
	MissingGithubTokenError = func() error {
		return eris.Errorf("Must either set GITHUB_TOKEN or set %s environment variable to true", skipChangelogGeneration)
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
	var repo string
	switch target {
	case glooDocGen:
		repo = glooOpenSourceRepo
	case glooEDocGen:
		repo = glooEnterpriseRepo
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

// Performs additional processing to generate changelog grouped and ordered by release version
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

// Fetches Gloo Open Source releases and orders them by version
func generateGlooChangelog() error {
	client := github.NewClient(nil)
	allReleases, err := getAllReleases(client, glooOpenSourceRepo)
	if err != nil {
		return err
	}

	minorReleaseMap, err := parseGlooReleases(allReleases, true)
	if err != nil {
		return err
	}
	printVersionOrderReleases(minorReleaseMap)
	return nil
}

// Fetches Gloo Enterprise releases and orders them by version
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
	enterpriseReleases, err := getAllReleases(client, glooEnterpriseRepo)
	if err != nil {
		return err
	}
	openSourceReleases, err := getAllReleases(client, glooOpenSourceRepo)
	if err != nil {
		return err
	}
	minorReleaseMap, err := parseGlooEReleases(enterpriseReleases, openSourceReleases)
	if err != nil {
		return err
	}

	printVersionOrderReleases(minorReleaseMap)
	return nil
}

// Fetches releases for repo from github
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

// Performs processing to generate a map of release version to the release notes
// This also pulls in open source gloo edge release notes and merges them with enterprise release notes
func parseGlooEReleases(enterpriseReleases, osReleases []*github.RepositoryRelease) (map[Version]string, error) {
	var minorReleaseMap = make(map[Version]string)
	for _, release := range enterpriseReleases {

		var releaseTag = release.GetTagName()
		version, err := versionutils.ParseVersion(releaseTag)

		if err != nil {
			return nil, err
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
		} else {
			continue
		}
		openSourceReleases, err := parseGlooReleases(osReleases, false)
		if err != nil {
			return nil, err
		}
		// Get release notes of the dependent open source gloo release version
		osReleaseNotes := openSourceReleases[Version(*depVersion)]
		body, err := parseEnterpriseNotes(release.GetBody(), osReleaseNotes, *depVersion)
		if err != nil {
			return nil, err
		}

		minorReleaseMap[minorVersion] = minorReleaseMap[minorVersion] + fmt.Sprintf("##### %s %s\n ", version.String(), glooOSSDescription) + body
	}

	return minorReleaseMap, nil
}

func parseEnterpriseNotes(enterpriseReleaseNotes, osReleaseNotes string, osVersion versionutils.Version) (string, error) {
	node := goldmark.DefaultParser().Parse(text.NewReader([]byte(enterpriseReleaseNotes)))
	//node.Dump([]byte(enterpriseReleaseNotes), 0)
	//buf := new(bytes.Buffer)
	//println(enterpriseReleaseNotes)
	//recursivelyPrintNode(node, "", enterpriseReleaseNotes)
	osReleaseBuf := []byte(osReleaseNotes)
	eReleaseBuf := []byte(enterpriseReleaseNotes)
	source := []byte(enterpriseReleaseNotes)
	osReleaseMap, err := parseOSNotes(osReleaseNotes)
	if err != nil {
		return "", err
	}
	var stop, pStop int
	for n, currentHeader := node.FirstChild(), ""; n != nil; n = n.NextSibling() {
		switch typedNode := n.(type) {
		case *ast.Paragraph:
			{
				if typedNode.FirstChild().Kind() == ast.KindEmphasis {
					currentHeader = string(typedNode.Text([]byte(enterpriseReleaseNotes)))
				} else {
					continue
				}
			}
		case *ast.List:
			{
				if typedNode.FirstChild().Kind() == ast.KindListItem {
					vLast := n.LastChild().FirstChild().Lines().At(0)
					pStop = vLast.Stop
					if stop == 0 {
						stop = pStop
					} else {
						stop = len(source)
					}

					if items := osReleaseMap[currentHeader]; len(items) != 0 {
						for i := 0; i < len(items); i++ {
							listItem := items[i]
							vToInsert := listItem.FirstChild().Lines().At(0)
							//fmt.Printf("INSERTING %s\n", osReleaseBuf[vToInsert.Start:vToInsert.Stop])
							osReleaseId := strings.ReplaceAll(osVersion.String(), ".", "")
							osRefLink := fmt.Sprintf("(From [OSS %s](/reference/changelog/open_source/#%s)) ", osVersion.String(), osReleaseId)
							step1 := append(source[:stop], []byte("\n- "+osRefLink)...)
							step2 := append(step1, osReleaseBuf[vToInsert.Start:vToInsert.Stop]...)
							source = append(step2, eReleaseBuf[pStop:]...)
							stop = len(step2)
						}
						delete(osReleaseMap, currentHeader)
					}
				}
			}
		}
	}
	for header, items := range osReleaseMap {
		sectionName := fmt.Sprintf("\n\n**%s**\n\n", header)
		step1 := append(source[:stop], []byte(sectionName)...)
		for i := 0; i < len(items); i++ {
			listItem := items[i]
			vToInsert := listItem.FirstChild().Lines().At(0)
			osReleaseId := strings.ReplaceAll(osVersion.String(), ".", "")
			osRefLink := fmt.Sprintf("(From [OSS %s](/reference/changelog/open_source/#%s)) ", osVersion.String(), osReleaseId)
			step2 := append(step1, []byte("\n- "+osRefLink)...)
			step3 := append(step2, osReleaseBuf[vToInsert.Start:vToInsert.Stop]...)
			source = append(step3, eReleaseBuf[pStop:]...)
			step1 = step3
			//fmt.Printf("adding - %s \n after \n %+v\n", osReleaseBuf[vToInsert.Start:vToInsert.Stop], enterpriseReleaseNotes[vLast.Start:vLast.Stop])
		}
	}
	return fmt.Sprintf("%s", source), nil
}

func parseOSNotes(osReleaseNotes string) (map[string][]*ast.ListItem, error) {
	node := goldmark.DefaultParser().Parse(text.NewReader([]byte(osReleaseNotes)))
	releaseNotes := make(map[string][]*ast.ListItem)

	for n, currentHeader := node.FirstChild(), ""; n != nil; n = n.NextSibling() {
		switch typedNode := n.(type) {
		case *ast.Paragraph:
			{
				switch typedNode.FirstChild().(type) {
				case *ast.Emphasis:
					currentHeader = string(typedNode.Text([]byte(osReleaseNotes)))
				default:
					continue
				}
			}
		case *ast.List:
			{
				switch typedNode.FirstChild().(type) {
				case *ast.ListItem:
					for l := typedNode.FirstChild(); l != nil; l = l.NextSibling() {
						releaseNotes[currentHeader] = append(releaseNotes[currentHeader], l.(*ast.ListItem))
					}
				}
			}
		}
	}
	return releaseNotes, nil
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

// Parses OSS Gloo Edge releases into correct format for printing
// If byMinorVersion is true, the version header (e.g. v1.5.9-beta8) is not included in the release notes body
func parseGlooReleases(releases []*github.RepositoryRelease, byMinorVersion bool) (map[Version]string, error) {
	var minorReleaseMap = make(map[Version]string)
	for _, release := range releases {
		var releaseTag = release.GetTagName()
		version, err := versionutils.ParseVersion(releaseTag)
		if err != nil {
			return nil, err
		}
		minorVersion := Version(*version)
		var header string
		// If byMinorVersion, we only want to include the release notes in the string and not the release header
		if byMinorVersion {
			header = fmt.Sprintf("##### %v\n", version.String())
			minorVersion.LabelVersion, minorVersion.Patch, minorVersion.Label = 0, 0, ""
		}
		minorReleaseMap[minorVersion] = minorReleaseMap[minorVersion] + header + release.GetBody()
	}

	return minorReleaseMap, nil
}

// Outputs changelogs in markdown format
func printVersionOrderReleases(minorReleaseMap map[Version]string) {
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
