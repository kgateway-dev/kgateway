package install

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/spf13/cobra"
)

func gatewayCmd(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gateway",
		Short: "install the Gloo Gateway on kubernetes",
		Long:  "requires kubectl to be installed",
		RunE: func(cmd *cobra.Command, args []string) error {

			// Get Gloo release version
			version, err := getGlooVersion(opts)
			if err != nil {
				return err
			}

			// Get location of Gloo install manifest
			manifestUri := fmt.Sprintf(constants.GlooHelmRepoTemplate, version)
			if manifestOverride := opts.Install.GlooManifestOverride; manifestOverride != "" {
				manifestUri = manifestOverride
			}

			// Pre-install step writes to k8s. Run only if this is not a dry run.
			if !opts.Install.DryRun {
				if err := preInstall(opts.Install.Namespace); err != nil {
					return errors.Wrapf(err, "pre-install failed")
				}
			}

			if err := installFromUri(manifestUri, opts, ""); err != nil {
				return errors.Wrapf(err, "installing gloo from helm")
			}
			return nil
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddInstallFlags(pflags, &opts.Install)
	return cmd
}
