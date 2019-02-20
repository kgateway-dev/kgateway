package install

import (
	"fmt"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/go-utils/errors"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/spf13/cobra"
)

func ingressCmd(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ingress",
		Short: "install the Gloo Ingress Controller on kubernetes",
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

			if err := installFromUri(manifestUri, opts, constants.IngressValuesFileName); err != nil {
				return errors.Wrapf(err, "installing ingress from manifest")
			}
			return nil
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddInstallFlags(pflags, &opts.Install)
	return cmd
}
