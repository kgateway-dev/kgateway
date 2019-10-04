package get

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/prerun"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/solo-io/go-utils/errors"
	"github.com/spf13/cobra"
)

var EmptyGetError = errors.New("please provide a subcommand")

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.GET_COMMAND.Use,
		Aliases: constants.GET_COMMAND.Aliases,
		Short:   constants.GET_COMMAND.Short,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := prerun.CallParentPrerun(cmd, args); err != nil {
				return err
			}
			if err := prerun.EnableConsulClients(opts.Get.Consul); err != nil {
				return err
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return EmptyGetError
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddMetadataFlags(pflags, &opts.Metadata)
	flagutils.AddOutputFlag(pflags, &opts.Top.Output)

	flagutils.AddConsulConfigFlags(cmd.PersistentFlags(), &opts.Get.Consul)

	cmd.AddCommand(VirtualService(opts))
	cmd.AddCommand(RouteTable(opts))
	cmd.AddCommand(Proxy(opts))
	cmd.AddCommand(Upstream(opts))
	cmd.AddCommand(UpstreamGroup(opts))
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
