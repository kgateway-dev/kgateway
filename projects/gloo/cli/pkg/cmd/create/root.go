package create

import (
	"io"
	"io/ioutil"
	"os"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/create/secret"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/common"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/solo-io/go-utils/errors"
	"github.com/spf13/cobra"
)

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"c"},
		Short:   "Create a Gloo resource",
		Long:    "Gloo resources be created from files (including stdin)",
		RunE: func(cmd *cobra.Command, args []string) error {
			var reader io.ReadCloser
			if opts.Top.File == "" {
				return errors.Errorf("create only takes a file")
			}
			if opts.Top.File == "-" {
				reader = os.Stdin
			} else {
				r, err := os.Open(opts.Top.File)
				if err != nil {
					return err
				}
				reader = r
			}
			yml, err := ioutil.ReadAll(reader)
			if err != nil {
				return err
			}
			return common.CreateAndPrintObject(yml, opts.Top.Output)
		},
	}
	flagutils.AddFileFlag(cmd.LocalFlags(), &opts.Top.File)
	flagutils.AddOutputFlag(cmd.PersistentFlags(), &opts.Top.Output)

	cmd.AddCommand(VirtualService(opts))
	cmd.AddCommand(Upstream(opts))
	cmd.AddCommand(secret.CreateCmd(opts))
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
