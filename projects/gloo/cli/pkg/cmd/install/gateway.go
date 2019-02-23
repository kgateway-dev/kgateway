package install

import (
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
			if err := installGloo(opts, constants.GatewayValuesFileName); err != nil {
				return errors.Wrapf(err, "installing gloo in gateway mode")
			}
			return nil
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddInstallFlags(pflags, &opts.Install)
	return cmd
}
