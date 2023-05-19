package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/dashboard"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/debug"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/demo"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/federation"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/istio"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/plugin"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"k8s.io/kubectl/pkg/cmd"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/add"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/check"
	check_crds "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/check-crds"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/create"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/del"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/edit"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/get"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/initpluginmanager"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/remove"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/route"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/upgrade"
	versioncmd "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/prerun"
	"github.com/solo-io/go-utils/cliutils"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/gateway"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/spf13/cobra"
)

func App(opts *options.Options, preRunFuncs []RunnableCommand, postRunFuncs []RunnableCommand, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {

	app := &cobra.Command{
		Use:   "glooctl",
		Short: "CLI for Gloo",
		Long: `glooctl is the unified CLI for Gloo.
	Find more information at https://solo.io`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// persistent pre run is be called after flag parsing
			// since this is the root of the cli app, it will be called regardless of the particular subcommand used
			for _, optFunc := range preRunFuncs {
				if err := optFunc(opts, cmd); err != nil {
					return err
				}
			}
			return nil
		},
		PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
			for _, optFunc := range postRunFuncs {
				if err := optFunc(opts, cmd); err != nil {
					return err
				}
			}
			return nil
		},
		SilenceUsage: true,
	}

	flagutils.AddKubeConfigFlag(app.PersistentFlags(), &opts.Top.KubeConfig)
	app.PersistentFlags()

	// Complete additional passed in setup
	cliutils.ApplyOptions(app, optionsFunc)

	// Handle glooctl plugins
	args := os.Args
	if len(args) > 1 {
		cmdPathPieces := args[1:]
		pluginHandler := cmd.NewDefaultPluginHandler(constants.ValidExtensionPrefixes)

		// If the given subcommand does not exist, look for a suitable plugin executable
		if _, _, err := app.Find(cmdPathPieces); err != nil {
			if err := cmd.HandlePluginCommand(pluginHandler, cmdPathPieces); err != nil {
				fmt.Printf("%v\n", err)
				os.Exit(1)
			}
		}
	}

	return app
}

func GlooCli() *cobra.Command {
	opts := &options.Options{
		Top: options.Top{
			Ctx: context.Background(),
		},
	}

	optionsFunc := func(app *cobra.Command) {
		pflags := app.PersistentFlags()
		pflags.BoolVarP(&opts.Top.Interactive, "interactive", "i", false, "use interactive mode")
		pflags.StringVarP(&opts.Top.ConfigFilePath, "config", "c", DefaultConfigPath, "set the path to the glooctl config file")
		flagutils.AddConsulConfigFlags(pflags, &opts.Top.Consul)
		flagutils.AddKubeContextFlag(pflags, &opts.Top.KubeContext)

		opts.Top.Ctx = context.WithValue(opts.Top.Ctx, "top", opts.Top.ContextAccessible)

		app.SuggestionsMinimumDistance = 1
		app.AddCommand(
			get.RootCmd(opts),
			del.RootCmd(opts),
			install.InstallCmd(opts),
			demo.RootCmd(opts),
			install.UninstallCmd(opts),
			add.RootCmd(opts),
			remove.RootCmd(opts),
			route.RootCmd(opts),
			create.RootCmd(opts),
			edit.RootCmd(opts),
			upgrade.RootCmd(opts),
			gateway.RootCmd(opts),
			check.RootCmd(opts),
			check_crds.RootCmd(opts),
			debug.RootCmd(opts),
			versioncmd.RootCmd(opts),
			dashboard.RootCmd(opts),
			federation.RootCmd(opts),
			plugin.RootCmd(opts),
			istio.RootCmd(opts),
			initpluginmanager.Command(context.Background()),
			completionCmd(),
		)
	}

	preRunFuncs := []RunnableCommand{
		// should make sure to read the config file first
		ReadConfigFile,
		prerun.SetKubeConfigEnv,
		prerun.SetPodNamespaceEnv,
		prerun.VersionMismatchWarning,
	}

	var postRunFuncs []RunnableCommand

	return App(opts, preRunFuncs, postRunFuncs, optionsFunc)
}

type RunnableCommand func(*options.Options, *cobra.Command) error
