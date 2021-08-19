package gateway

import (
	"fmt"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/xdsinspection"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

func servedConfigCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "served-config",
		Short: "dump Envoy config being served by the Gloo xDS server",
		RunE: func(cmd *cobra.Command, args []string) error {
			servedConfig, err := printGlooXdsDump(opts)
			if err != nil {
				return err
			}
			fmt.Printf("%v", servedConfig)
			return nil
		},
	}
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}

func printGlooXdsDump(opts *options.Options) (string, error) {
	dump, err := xdsinspection.GetGlooXdsDump(opts.Top.Ctx, opts.Proxy.Name, opts.Metadata.GetNamespace(), true)
	if err != nil {
		return "", err
	}
	return dump.String(), nil
}
