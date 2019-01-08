package install

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "install gloo on different platforms",
		Long:  "choose which system to install Gloo onto. options include: kubernetes",
	}
	cmd.AddCommand(KubeCmd(opts))
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
