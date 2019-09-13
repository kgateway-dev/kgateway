package install

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/go-utils/errors"
	"github.com/spf13/cobra"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

func gatewayCmd(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "gateway",
		Short:  "install the Gloo Gateway on kubernetes",
		Long:   "requires kubectl to be installed",
		PreRun: setVerboseMode(opts),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := installGloo(opts, constants.GatewayValuesFileName); err != nil {
				return errors.Wrapf(err, "installing gloo in gateway mode")
			}
			return nil
		},
	}

	pflags := cmd.PersistentFlags()
	flagutils.AddInstallFlags(pflags, &opts.Install)

	cmd.AddCommand(enterpriseCmd(opts))

	return cmd
}
