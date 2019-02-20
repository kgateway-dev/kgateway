package install

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/solo-io/go-utils/errors"
	"github.com/spf13/cobra"
)

func InstallCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   constants.INSTALL_COMMAND.Use,
		Short: constants.INSTALL_COMMAND.Short,
		Long:  constants.INSTALL_COMMAND.Long,
	}
	cmd.AddCommand(gatewayCmd(opts))
	cmd.AddCommand(ingressCmd(opts))
	cmd.AddCommand(knativeCmd(opts))
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}

func UninstallCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   constants.UNINSTALL_COMMAND.Use,
		Short: constants.UNINSTALL_COMMAND.Short,
		Long:  constants.UNINSTALL_COMMAND.Long,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := kubectl(nil, "delete", "namespace", opts.Uninstall.Namespace); err != nil {
				return errors.Wrapf(err, "delete gloo failed")
			}
			knativeExists, isOurInstall, err := knativeInstalled()
			if err != nil {
				return errors.Wrapf(err, "finding knative installation")
			}
			if knativeExists && isOurInstall {
				if err := kubectl(nil, "delete", "namespace", constants.KnativeServingNamespace); err != nil {
					return errors.Wrapf(err, "delete knative failed")
				}
			}
			return nil
		},
	}

	pFlags := cmd.PersistentFlags()
	flagutils.AddUninstallFlags(pFlags, &opts.Uninstall)

	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
