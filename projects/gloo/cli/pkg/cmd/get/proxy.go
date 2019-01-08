package get

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/common"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/spf13/cobra"
)

func Proxy(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "proxy",
		Aliases: []string{"p", "proxies"},
		Short:   "read a proxy or list proxies in a namespace",
		Long:    "usage: glooctl get proxy",
		RunE: func(cmd *cobra.Command, args []string) error {
			proxyList, err := common.GetProxies(common.GetName(args, opts), opts)
			if err != nil {
				return err
			}
			helpers.PrintProxies(proxyList, opts.Top.Output)
			return nil
		},
	}
	return cmd
}
