package ui

import (
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

const glooUiPath = "/overview/"

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   constants.UI_COMMAND.Use,
		Short: constants.UI_COMMAND.Short,
		Long:  constants.UI_COMMAND.Long,
		RunE: func(cmd *cobra.Command, args []string) error {
			port := strconv.Itoa(int(defaults.HttpPort))

			kubectl := exec.Command("kubectl", "port-forward", "-n", opts.Metadata.Namespace,
				"deployment/api-server", port)

			err := cliutil.Initialize()
			if err != nil {
				return err
			}
			kubectl.Stdout = cliutil.GetLogger()
			kubectl.Stderr = cliutil.GetLogger()

			err = kubectl.Start()
			if err != nil {
				return err
			}
			defer kubectl.Wait()

			err = browser.OpenURL("http://localhost:" + port + glooUiPath)
			if err != nil {
				return err
			}

			return nil
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddMetadataFlags(pflags, &opts.Metadata)
	flagutils.AddOutputFlag(pflags, &opts.Top.Output)

	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
