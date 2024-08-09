package license

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/spf13/cobra"
)

func RootCmd(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "license",
		Aliases: []string{"l"},
		Short:   "subcommands for interacting with the license",
	}
	cmd.AddCommand(License(opts))
	return cmd
}
