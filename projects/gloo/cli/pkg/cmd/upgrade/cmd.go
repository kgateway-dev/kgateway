package upgrade

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"

	"github.com/solo-io/go-utils/cliutils"

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
		"to download. Specify a git tag corresponding to the desired version of glooctl.")
	cmd.PersistentFlags().StringVar(&opts.Upgrade.DownloadPath, "path", "", "Desired path for your "+
		"upgraded glooctl binary. Defaults to the location of your currently executing binary.")
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}

func upgradeGlooCtl(ctx context.Context, upgrade options.Upgrade) error {
	release, err := getRelease(ctx, upgrade.ReleaseTag)
	if err != nil {
		return errors.Wrapf(err, "getting release '%v' from solo-io/gloo repository", upgrade.ReleaseTag)
	}

	glooctlBinaryName := fmt.Sprintf("glooctl-%v-amd64", runtime.GOOS)

	fmt.Printf("downloading %v from release tag %v\n", glooctlBinaryName, release.GetTagName())

	var downloadUrl string
	for _, asset := range release.Assets {
		if asset.GetName() == glooctlBinaryName {
			downloadUrl = asset.GetBrowserDownloadURL()
		}
	}
	if downloadUrl == "" {
		return errors.Errorf("could not find asset %v in release %v", glooctlBinaryName, release.GetTagName())
	}

	if err := downloadAsset(downloadUrl, upgrade.DownloadPath); err != nil {
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

func getRelease(ctx context.Context, tag string) (*github.RepositoryRelease, error) {
	g := github.NewClient(nil)
	if tag == "latest" {
		release, _, err := g.Repositories.GetLatestRelease(ctx, "solo-io", "gloo")
		return release, err
	}
	release, _, err := g.Repositories.GetReleaseByTag(ctx, "solo-io", "gloo", tag)
	return release, err
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
