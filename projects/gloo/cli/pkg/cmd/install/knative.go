package install

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/go-utils/errors"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/spf13/cobra"
)

func knativeCmd(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "knative",
		Short: "install Knative with Gloo on kubernetes",
		Long:  "requires kubectl to be installed",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := installGloo(opts, constants.KnativeValuesFileName); err != nil {
				return errors.Wrapf(err, "installing gloo in knative mode")
			}
			return nil
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddInstallFlags(pflags, &opts.Install)
	return cmd
}
