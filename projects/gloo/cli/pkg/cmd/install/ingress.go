package install

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/go-utils/errors"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/spf13/cobra"
)

func ingressCmd(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "ingress",
		Short:  "install the Gloo Ingress Controller on kubernetes",
		Long:   "requires kubectl to be installed",
		PreRun: setVerboseMode(opts),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := installGloo(opts, constants.IngressValuesFileName); err != nil {
				return errors.Wrapf(err, "installing gloo in ingress mode")
			}
			return nil
		},
	}
	return cmd
}
