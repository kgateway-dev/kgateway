package v2

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type Options struct {
	Namespace string
	Values    []string
	Set       []string
	DryRun    string
	Gateway   bool
}

func addFlags(set *pflag.FlagSet, opts *Options) {
	set.StringVarP(&opts.Namespace, "namespace", "n", defaults.GlooSystem, "namespace in which Gloo is installed")
	set.StringSliceVar(&opts.Values, "values", nil, "path to a helm values file (can be repeated or comma separated list of values)")
	set.StringSliceVar(&opts.Set, "set", nil, "directly set values for the gloo gateway helm chart (can be repeated or comma separated list of values)")
	set.StringVar(&opts.DryRun, "dry-run", "", "print the generated kubernetes manifest to stdout")
	set.BoolVarP(&opts.Gateway, "gateway", "g", false, "install the default gloo gateway proxy, (default false)")
}

func InstallCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {

	installOpts := &Options{}
	cmd := &cobra.Command{
		Use:   constants.INSTALL_COMMAND.Use,
		Short: constants.INSTALL_COMMAND.Short,
		Long:  constants.INSTALL_COMMAND.Long,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Default to Gloo Gateway V2
			return install(opts, installOpts)
		},
	}
	cliutils.ApplyOptions(cmd, optionsFunc)
	addFlags(cmd.Flags(), installOpts)

	pFlags := cmd.PersistentFlags()
	flagutils.AddVerboseFlag(pFlags, opts)
	return cmd
}

func UninstallCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {

	installOpts := &Options{}
	cmd := &cobra.Command{
		Use:   constants.UNINSTALL_COMMAND.Use,
		Short: constants.UNINSTALL_COMMAND.Short,
		Long:  constants.UNINSTALL_COMMAND.Long,
		RunE: func(cmd *cobra.Command, args []string) error {
			return uninstall(opts, installOpts)
		},
	}

	cliutils.ApplyOptions(cmd, optionsFunc)
	addFlags(cmd.Flags(), installOpts)

	pFlags := cmd.PersistentFlags()
	flagutils.AddVerboseFlag(pFlags, opts)
	return cmd
}
