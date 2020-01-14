package ui

import (
	"io"
	"os"
	"os/exec"
	"strconv"

	"github.com/pkg/browser"
	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.UI_COMMAND.Use,
		Aliases: constants.UI_COMMAND.Aliases,
		Short:   constants.UI_COMMAND.Short,
		Long:    constants.UI_COMMAND.Long,
		RunE: func(cmd *cobra.Command, args []string) error {
			port := strconv.Itoa(int(defaults.HttpPort))

			portFwd := exec.Command("kubectl", "port-forward", "-n", opts.Metadata.Namespace,
				"deployment/api-server", port)

			err := cliutil.Initialize()
			if err != nil {
				return err
			}
			logger := cliutil.GetLogger()

			portFwd.Stderr = io.MultiWriter(logger, os.Stderr)
			if opts.Top.Verbose {
				portFwd.Stdout = io.MultiWriter(logger, os.Stdout)
			} else {
				portFwd.Stdout = logger
			}

			if err := portFwd.Start(); err != nil {
				return err
			}
			defer portFwd.Wait()

			if err := browser.OpenURL("http://localhost:" + port); err != nil {
				return err
			}

			return nil
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddNamespaceFlag(pflags, &opts.Metadata.Namespace)
	flagutils.AddVerboseFlag(pflags, opts)

	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
