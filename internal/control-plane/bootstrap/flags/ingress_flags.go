package flags

import (
	"github.com/solo-io/gloo/internal/control-plane/bootstrap"
	"github.com/spf13/cobra"
)

func AddIngressFlags(cmd *cobra.Command, opts *bootstrap.Options) {
	// TODO ingress.bind-adress
	cmd.PersistentFlags().StringVar(&opts.IngressOptions.BindAddress, "envoy.bind-adress", "", "The address that the ingress envoy should bind to.")
}
