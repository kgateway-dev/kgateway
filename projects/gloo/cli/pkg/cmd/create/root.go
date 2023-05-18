package create

import (
	"io"
	"os"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/create/authconfig"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/prerun"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/create/secret"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/common"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

const EmptyCreateError = "please provide a file flag or subcommand"

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.CREATE_COMMAND.Use,
		Aliases: constants.CREATE_COMMAND.Aliases,
		Short:   constants.CREATE_COMMAND.Short,
		Long:    constants.CREATE_COMMAND.Long,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := prerun.CallParentPrerun(cmd, args); err != nil {
				return err
			}
			if err := prerun.HarmonizeDryRunAndOutputFormat(opts, cmd); err != nil {
				return err
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			var reader io.ReadCloser
			if opts.Top.File == "" {
				return eris.Errorf(EmptyCreateError)
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
			yml, err := io.ReadAll(reader)
			if err != nil {
				return err
			}
			return common.CreateAndPrintObject(opts.Top.Ctx, yml, opts.Top.Output, opts.Metadata.GetNamespace())
		},
	}
	flagutils.AddFileFlag(cmd.LocalFlags(), &opts.Top.File)
	flagutils.AddOutputFlag(cmd.PersistentFlags(), &opts.Top.Output)
	flagutils.AddMetadataFlags(cmd.PersistentFlags(), &opts.Metadata)
	flagutils.AddDryRunFlag(cmd.PersistentFlags(), &opts.Create.DryRun)

	cmd.AddCommand(VSCreate(opts))
	cmd.AddCommand(Upstream(opts))
	cmd.AddCommand(UpstreamGroup(opts))
	cmd.AddCommand(secret.CreateCmd(opts))
	cmd.AddCommand(authconfig.AuthConfigCreate(opts))
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
