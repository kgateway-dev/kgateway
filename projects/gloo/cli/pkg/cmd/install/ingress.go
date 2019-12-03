package install

import (
	"github.com/solo-io/go-utils/errors"
	"helm.sh/helm/v3/pkg/chartutil"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/spf13/cobra"
)

func ingressCmd(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "ingress",
		Short:  "install the Gloo Ingress Controller on Kubernetes",
		Long:   "requires kubectl to be installed",
		PreRun: setVerboseMode(opts),
		RunE: func(cmd *cobra.Command, args []string) error {

			ingressOverrides, err := chartutil.ReadValues([]byte(IngressValues))
			if err != nil {
				return errors.Wrapf(err, "parsing override values for ingress mode")
			}

			if err := Install(&opts.Install, ingressOverrides, false); err != nil {
				return errors.Wrapf(err, "installing gloo in ingress mode")
			}

			return nil
		},
	}
	return cmd
}
