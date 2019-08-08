package cmd

import (
	"context"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/remove"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/route"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/add"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/create"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/del"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/edit"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/get"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/upgrade"
	"github.com/solo-io/go-utils/cliutils"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/gateway"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/spf13/cobra"
)

var versionTemplate = `{{with .Name}}{{printf "%s community edition " .}}{{end}}{{printf "version %s" .Version}}
`

// TODO PREMERGE: pass preRunFuncs for harmonizing config, will be a []func(*options.Options) error
func App(version string, opts *options.Options, preRunFuncs []PreRunFunc, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {

	app := &cobra.Command{
		Use:   "glooctl",
		Short: "CLI for Gloo",
		Long: `glooctl is the unified CLI for Gloo.
	Find more information at https://solo.io`,
		Version: version,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// persistent pre run is be called after flag parsing
			// since this is the root of the cli app, it will be called regardless of the particular subcommand used
			for _, optFunc := range preRunFuncs {
				if err := optFunc(opts); err != nil {
					return err
				}
			}
			return nil
		},
	}

	// Complete additional passed in setup
	cliutils.ApplyOptions(app, optionsFunc)

	app.SetVersionTemplate(versionTemplate)

	return app
}

func GlooCli(version string) *cobra.Command {
	opts := &options.Options{
		Top: options.Top{
			Ctx: context.Background(),
		},
	}

	optionsFunc := func(app *cobra.Command) {
		pflags := app.PersistentFlags()
		pflags.BoolVarP(&opts.Top.Interactive, "interactive", "i", false, "use interactive mode")

		app.SuggestionsMinimumDistance = 1
		app.AddCommand(
			get.RootCmd(opts),
			del.RootCmd(opts),
			install.InstallCmd(opts),
			install.UninstallCmd(opts),
			add.RootCmd(opts),
			remove.RootCmd(opts),
			route.RootCmd(opts),
			create.RootCmd(opts),
			edit.RootCmd(opts),
			upgrade.RootCmd(opts),
			gateway.RootCmd(opts),
			completionCmd(),
		)
	}

	preRunFuncs := []PreRunFunc{
		HarmonizeDryRunAndOutputFormat,
	}

	return App(version, opts, preRunFuncs, optionsFunc)
}

// TODO move to cliutils
type PreRunFunc func(*options.Options) error

func HarmonizeDryRunAndOutputFormat(opts *options.Options) error {
	// in order to allow table output by default, and meaningful dry runs we need to override the output default
	// enforcing this in the PersistentPreRun saves us from having to do so in any new printers or output types
	if opts.Create.DryRun && opts.Top.Output == printers.TABLE {
		opts.Top.Output = printers.KUBE_YAML
	}
	return nil
}
