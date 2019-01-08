package cmd

import (
	"context"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/add"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/create"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/del"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/get"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/upgrade"
	"github.com/solo-io/go-utils/cliutils"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/gateway"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/spf13/cobra"
)

func App(version string, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {

	app := &cobra.Command{
		Use:   "glooctl",
		Short: "CLI for Gloo",
		Long: `glooctl is the unified CLI for Gloo.
	Find more information at https://solo.io`,
		Version: version,
	}

	// Complete additional passed in setup
	cliutils.ApplyOptions(app, optionsFunc)

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
			install.RootCmd(opts),
			add.RootCmd(opts),
			create.RootCmd(opts),
			upgrade.RootCmd(opts),
			gateway.RootCmd(opts),
		)
	}

	return App(version, optionsFunc)
}
