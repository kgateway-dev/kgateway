package install

import (
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/spf13/cobra"
)

func gatewayCmd(opts *options.Options) *cobra.Command {
	const glooGatewayUrlTemplate = "https://github.com/solo-io/gloo/releases/download/v%s/gloo-gateway.yaml"
	cmd := &cobra.Command{
		Use:   "gateway",
		Short: "install the Gloo Gateway on kubernetes",
		Long:  "requires kubectl to be installed",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := preInstall(); err != nil {
				return errors.Wrapf(err, "pre-install failed")
			}
			if err := installFromUri(opts, opts.Install.GlooManifestOverride, glooGatewayUrlTemplate); err != nil {
				return errors.Wrapf(err, "installing ingress from manifest")
			}
			return nil
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddInstallFlags(pflags, &opts.Install)
	return cmd
}
