package upgrade

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"

	update "github.com/inconshreveable/go-update"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"

	"github.com/solo-io/go-utils/cliutils"
	"github.com/solo-io/go-utils/versionutils"

	"github.com/google/go-github/github"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/spf13/cobra"
)

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.UPGRADE_COMMAND.Use,
		Aliases: constants.UPGRADE_COMMAND.Aliases,
		Short:   constants.UPGRADE_COMMAND.Short,
		RunE: func(cmd *cobra.Command, args []string) error {
			return upgradeGlooCtl(opts.Top.Ctx, opts.Upgrade)
		},
	}

	cmd.PersistentFlags().StringVar(&opts.Upgrade.ReleaseTag, "release", "latest", "Which glooctl release "+
		"to download. Specify a release tag corresponding to the desired version of glooctl,"+
		"\"experimental\" to use the version currently under development,"+
		" or a major+minor release like v1.10.x to get the most recent patch version.")
	cmd.PersistentFlags().StringVar(&opts.Upgrade.DownloadPath, "path", "", "Desired path for your "+
		"upgraded glooctl binary. Defaults to the location of your currently executing binary.")
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}

var knownTags = map[string]struct{}{"experimental": struct{}{}, "latest": struct{}{}}

// timeoutseconds for our http client. This should move along with
// the client options to a higher place in code and become merely a default setting.
const timeoutSeconds = 10

func upgradeGlooCtl(ctx context.Context, upgrade options.Upgrade) error {
	// probably should propagate this up higher so we can set option on glooctl http clients in general
	httpClient := &http.Client{
		Timeout: time.Duration(timeoutSeconds) * time.Second,
	}

	glooctlBinaryName := ""
	if runtime.GOOS != "windows" {
		glooctlBinaryName = fmt.Sprintf("glooctl-%v-%v", runtime.GOOS, runtime.GOARCH)
	} else {
		//Windows binaries have .exe in asset name
		glooctlBinaryName = fmt.Sprintf("glooctl-%v-%v%s", runtime.GOOS, runtime.GOARCH, ".exe")
	}

	release, err := getReleaseWithAsset(ctx, httpClient, upgrade.ReleaseTag, glooctlBinaryName)
	if err != nil {
		return errors.Wrapf(err, "getting release '%v' from solo-io/gloo repository", upgrade.ReleaseTag)
	}

	fmt.Printf("downloading %v from release tag %v\n", glooctlBinaryName, release.GetTagName())

	asset := tryGetAssetWithName(release, glooctlBinaryName)
	if asset == nil {
		return errors.Errorf("could not find asset %v in release %v", glooctlBinaryName, release.GetTagName())
	}

	if err := downloadAsset(asset.GetBrowserDownloadURL(), upgrade.DownloadPath); err != nil {
		return errors.Wrapf(err, "downloading asset %v", glooctlBinaryName)
	}

	downloadPath := upgrade.DownloadPath
	if downloadPath == "" {
		downloadPath, err = os.Executable()
		if err != nil {
			return errors.Wrapf(err, "getting currently executing binary path")
		}
	}

	fmt.Printf("successfully downloaded and installed glooctl version %v to %v\n", release.GetTagName(), downloadPath)
	return nil
}

const maxPages = 10

// getReleaseWithAsset attempts to get a release per the given tag with the desired asset form of expectedAssetName
func getReleaseWithAsset(ctx context.Context, httpClient *http.Client, tag string, expectedAssetName string) (*github.RepositoryRelease, error) {

	g := github.NewClient(httpClient)

	// For testing purposes mainly but potentially could be used
	// for specific on prem managed releases.
	baseURL := ctx.Value("githubURL")
	if baseURLStr, ok := baseURL.(string); ok {
		g.BaseURL, _ = url.Parse(baseURLStr)
	}

	if versionutils.MatchesRegex(tag) {
		release, _, err := g.Repositories.GetReleaseByTag(ctx, "solo-io", "gloo", tag)
		return release, err
	}

	regex := regexp.MustCompile("(v[0-9]+[.][0-9]+)")
	specifiedVersion := regex.FindString(tag)
	if specifiedVersion != "" {
		// not using logger as is not used previously
		fmt.Println("searching for release version", specifiedVersion)
	} else if _, ok := knownTags[tag]; !ok {
		return nil, fmt.Errorf("unknown release specification %s", tag)
	}

	var largestValidSemVer versionutils.Version
	var candidateRelease *github.RepositoryRelease
	// inexact version requested may be prerelease and not have assets
	// We do assume that within a minor version we use monotonically increasing patch numbers
	// We also assume that the first release that is not strict semver is technically the largest
	for i := 0; i < maxPages; i++ {
		// Get the next page of
		listOpts := github.ListOptions{PerPage: 10, Page: i} // max per request
		releases, _, err := g.Repositories.ListReleases(ctx, "solo-io", "gloo", &listOpts)
		if err != nil {
			return nil, errors.Wrapf(err, "error listing releases")
		}

		for _, release := range releases {
			v, err := versionutils.ParseVersion(*release.Name)
			if err != nil {
				continue
			}

			if v.Major != 1{
				continue
			}

			// We only consider releases that have assets to download.
			// More expensive to do this call than to check version infos.
			if tryGetAssetWithName(release, expectedAssetName) == nil {
				continue
			}

			// we have a valid versioned release along with a release asset at this point
			// Now we must check if it fits any of the criteria.

			// Track best candidate that is fully valid semver (serves the latest)
			// Dont respect non strictsemver for storage purposes (v.Label)
			if v.Label == "" && (*v).MustIsGreaterThan(largestValidSemVer) {
				largestValidSemVer = (*v)
				candidateRelease = release
			}

			// either a major-minor was specified something of the form v%d.%d
			// or are searching for latest stable and have found the most recent
			// experimental and are now searching for a conforming release
			if specifiedVersion != "" {
				// take the first valid from this version
				// as we assume increasing ordering
				if strings.HasPrefix(v.String(), specifiedVersion) {
					return release, nil
				}
				continue
			}

			// If not strict semver we dont need to analyze our special tags
			if v.Label == "" {
				continue
			}

			// looking for a something like latest or experimental
			// and need to determine minor / major entirely as we have nothing to go on from the input
			if tag == "experimental" {
				// the user has requested the experimental release and this is the largest non-strict semver
				//  therefore web return this value.
				return release, nil
			}
			if tag == "latest" {
				stableMinor := v.Minor - 1
				// for major increase this will be pretty bad performance wise
				// but at least its valid and only bad for a single release cycle
				if stableMinor >= 0 {
					specifiedVersion = fmt.Sprintf("v%d.%d", v.Major, stableMinor)
					if candidateRelease == nil {
						continue
					}
					candidateV, _ := versionutils.ParseVersion(*candidateRelease.Name)
					// we may have already captured the latest stable
					if strings.HasPrefix(specifiedVersion, candidateV.String()) {
						return release, nil
					}
				}
			}
		}
	}

	// edge case for major version increase
	if tag == "latest" {
		return candidateRelease, nil
	}

	return nil, errors.Errorf(errorNotFoundString)

}

const errorNotFoundString = "couldn't find any recent release with the desired asset"

func tryGetAssetWithName(release *github.RepositoryRelease, expectedAssetName string) *github.ReleaseAsset {
	for _, asset := range release.Assets {
		if asset.GetName() == expectedAssetName {
			return &asset
		}
	}
	return nil
}

func downloadAsset(downloadUrl string, destFile string) error {
	res, err := http.Get(downloadUrl)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if err := update.Apply(res.Body, update.Options{
		TargetPath: destFile,
	}); err != nil {
		return err
	}
	return nil
}
