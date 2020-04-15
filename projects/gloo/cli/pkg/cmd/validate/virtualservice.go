package validate

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/surveyutils"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

func ValidateVirtualService(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "virtualservice",
		Aliases: []string{"vs"},
		Short:   "validate virtualservice config",
		Long: "Validates virtual service configuration in a vacuum as being valid or invalid. Keep in mind that a virtual " +
			"service can be invalid in the context of other, pre-existing virtual services (e.g., conflicting domains)." +
			"\n\n" +
			"Usage: `glooctl validate virtualservice [--file virtual-service(s)]`",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if opts.Top.Interactive {
				if err := surveyutils.SelectVirtualServiceInteractive(opts); err != nil {
					return err
				}
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil //sortRoutes(opts)

			// TODO(kdorosh) consider expanding command to allow any kind of resources to be provided and use them
			// in the snapshot to validate before application.

			// do we want to use all the same validation webhook logic here...?

			// connect to the live gloo and use that grpc validation server result from glooctl
			// add a plugin to do the validation in a gloo plugin and add the status
			// validation runs same translator so if its in a plugin then it will be live too (in gateway admission hook)

		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddOutputFlag(pflags, &opts.Top.Output)
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
