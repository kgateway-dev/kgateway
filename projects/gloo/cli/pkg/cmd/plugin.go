package cmd

import (
	"context"
	"os"
	"os/exec"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func Plugin(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {

	plugin := &cobra.Command{
		Use:   "pi",
		Short: "Execute a plugin for glooctl",
		Long:  `Execute a plugin for glooctl. TODO doc how they become accessible`,
		//PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		//	// persistent pre run is be called after flag parsing
		//	// since this is the root of the cli app, it will be called regardless of the particular subcommand used
		//	for _, optFunc := range preRunFuncs {
		//		if err := optFunc(opts, cmd); err != nil {
		//			return err
		//		}
		//	}
		//	return nil
		//},
		RunE: func(cmd *cobra.Command, args []string) error {
			contextutils.LoggerFrom(context.TODO()).Infow("running", zap.Any("args", args))

			if len(args) < 1 {
				panic("todo")
			}

			pi := exec.Command(args[0], args[1:]...)
			pi.Stdout = os.Stdout
			return pi.Run()
		},
	}

	flagutils.AddKubeConfigFlag(plugin.PersistentFlags(), &opts.Top.KubeConfig)
	plugin.PersistentFlags()

	cliutils.ApplyOptions(plugin, optionsFunc)

	return plugin
}

type Input struct {
	name string
	args []string
}

// empty for now, just use stdin/out
type Output struct{}
