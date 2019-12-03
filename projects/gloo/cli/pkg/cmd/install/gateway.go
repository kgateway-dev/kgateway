package install

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/go-utils/errors"
	"github.com/spf13/cobra"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

func gatewayCmd(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gateway",
		Short: "install the Gloo Gateway on Kubernetes",
		Long:  "requires kubectl to be installed",
		// TODO(helm3): wire this up properly
		PreRun: setVerboseMode(opts),

		RunE: func(cmd *cobra.Command, args []string) error {
			if err := Install(&opts.Install, nil, false); err != nil {
				return errors.Wrapf(err, "installing gloo in gateway mode")
			}
			return nil
		},
	}

	cmd.AddCommand(enterpriseCmd(opts))

	return cmd
}
