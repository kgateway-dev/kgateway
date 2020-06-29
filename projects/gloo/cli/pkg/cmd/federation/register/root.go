package register

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"

	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.REGISTER_COMMAND.Use,
		Aliases: constants.REGISTER_COMMAND.Aliases,
		Short:   constants.REGISTER_COMMAND.Short,
		Long:    constants.REGISTER_COMMAND.Long,
		RunE: func(cmd *cobra.Command, args []string) error {
			return Register(opts)
		},
	}

	pflags := cmd.PersistentFlags()
	flagutils.AddRegisterFlags(pflags, &opts.Hub.Cluster.Register)
	// this flag is mainly for demo, testing, and debugging purposes
	pflags.Lookup(flagutils.LocalClusterDomainOverride).Hidden = true
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
