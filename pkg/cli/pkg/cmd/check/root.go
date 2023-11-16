package check

import (
	"bytes"
	"context"
	"errors"

	"github.com/rotisserie/eris"
	v2 "github.com/solo-io/gloo/v2/pkg/cli/pkg/cmd/check/internal/v2"
	"github.com/solo-io/gloo/v2/pkg/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/v2/pkg/cli/pkg/constants"
	"github.com/solo-io/gloo/v2/pkg/cli/pkg/flagutils"
	"github.com/solo-io/gloo/v2/pkg/cli/pkg/printers"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

var (
	CrdNotFoundErr = func(crdName string) error {
		return eris.Errorf("%s CRD has not been registered", crdName)
	}

	// printer printers.P
)

// contains method
func doesNotContain(arr []string, str string) bool {
	for _, a := range arr {
		if a == str {
			return false
		}
	}
	return true
}

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   constants.CHECK_COMMAND.Use,
		Short: constants.CHECK_COMMAND.Short,
		Long:  "usage: glooctl check [-o FORMAT]",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithCancel(opts.Top.Ctx)

			if opts.Check.CheckTimeout != 0 {
				ctx, cancel = context.WithTimeout(opts.Top.Ctx, opts.Check.CheckTimeout)
			}
			defer cancel()

			if !opts.Top.Output.IsTable() && !opts.Top.Output.IsJSON() {
				return errors.New("Invalid output type. Only table (default) and json are supported.")
			}

			printer := printers.P{OutputType: opts.Top.Output}
			printer.CheckResult = printer.NewCheckResult()
			err := v2.Check(ctx, printer, opts)

			if err != nil {
				// Not returning error here because this shouldn't propagate as a standard CLI error, which prints usage.
				if opts.Top.Output.IsTable() {
					return err
				}
			} else {
				printer.AppendMessage("No problems detected.")
			}

			// CheckMulticlusterResources(ctx, printer, opts)

			if opts.Top.Output.IsJSON() {
				printer.PrintChecks(new(bytes.Buffer))
			}

			return nil
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddCheckOutputFlag(pflags, &opts.Top.Output)
	flagutils.AddNamespaceFlag(pflags, &opts.Top.Namespace)
	flagutils.AddResourceNamespaceFlag(pflags, &opts.Top.ResourceNamespaces)
	flagutils.AddExcludeCheckFlag(pflags, &opts.Top.CheckName)
	flagutils.AddReadOnlyFlag(pflags, &opts.Top.ReadOnly)
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
