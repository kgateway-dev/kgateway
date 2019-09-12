package install

import (
	"fmt"

	"github.com/solo-io/gloo/pkg/cliutil/install"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

func InstallCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   constants.INSTALL_COMMAND.Use,
		Short: constants.INSTALL_COMMAND.Short,
		Long:  constants.INSTALL_COMMAND.Long,
	}
	cmd.AddCommand(
		gatewayCmd(opts),
		ingressCmd(opts),
		knativeCmd(opts),
	)
	cliutils.ApplyOptions(cmd, optionsFunc)
	flagutils.AddVerboseFlag(cmd.PersistentFlags(), opts)
	return cmd
}

func UninstallCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:    constants.UNINSTALL_COMMAND.Use,
		Short:  constants.UNINSTALL_COMMAND.Short,
		Long:   constants.UNINSTALL_COMMAND.Long,
		PreRun: setVerboseMode(opts),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Uninstalling Gloo...\n")
			if err := UninstallGloo(opts, &install.CmdKubectl{}); err != nil {
				return err
			}
			fmt.Printf("\nGloo was successfully uninstalled.\n")
			return nil
		},
	}

	pFlags := cmd.PersistentFlags()
	flagutils.AddUninstallFlags(pFlags, &opts.Uninstall)

	cliutils.ApplyOptions(cmd, optionsFunc)
	flagutils.AddVerboseFlag(cmd.PersistentFlags(), opts)
	return cmd
}

func setVerboseMode(opts *options.Options) func(cmd *cobra.Command, args []string) {
	return func(cmd *cobra.Command, args []string) {
		install.SetVerbose(opts.Top.Verbose)
	}
}
