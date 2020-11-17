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
		glooFedCmd(opts),
	)
	cliutils.ApplyOptions(cmd, optionsFunc)

	pFlags := cmd.PersistentFlags()
	flagutils.AddVerboseFlag(pFlags, opts)
	return cmd
}

func UninstallCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:    constants.UNINSTALL_COMMAND.Use,
		Short:  constants.UNINSTALL_COMMAND.Short,
		Long:   constants.UNINSTALL_COMMAND.Long,
		PreRun: setVerboseMode(opts),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Uninstalling Gloo Edge...\n")
			if err := Uninstall(opts, &install.CmdKubectl{}, Gloo); err != nil {
				return err
			}
			fmt.Printf("\nGloo was successfully uninstalled.\n")
			return nil
		},
	}

	cmd.AddCommand(UninstallGlooFedCmd(opts))

	flagutils.AddGlooUninstallFlags(cmd.Flags(), &opts.Uninstall.GlooUninstall)
	cliutils.ApplyOptions(cmd, optionsFunc)
	flagutils.AddVerboseFlag(cmd.PersistentFlags(), opts)

	return cmd
}

func UninstallGlooFedCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:    constants.UNINSTALL_GLOO_FED_COMMAND.Use,
		Short:  constants.UNINSTALL_GLOO_FED_COMMAND.Short,
		Long:   constants.UNINSTALL_GLOO_FED_COMMAND.Long,
		PreRun: setVerboseMode(opts),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Uninstalling Gloo Edge Federation...\n")
			if err := Uninstall(opts, &install.CmdKubectl{}, Federation); err != nil {
				return err
			}
			fmt.Printf("\nGloo Federation was successfully uninstalled.\n")
			return nil
		},
	}

	flagutils.AddGlooFedUninstallFlags(cmd.PersistentFlags(), &opts.Uninstall.FedUninstall)
	return cmd
}

func setVerboseMode(opts *options.Options) func(cmd *cobra.Command, args []string) {
	return func(cmd *cobra.Command, args []string) {
		install.SetVerbose(opts.Top.Verbose) // Sets kubectl verbose flag
		setVerbose(opts.Top.Verbose)         // Sets helm library's debug flag
	}
}
