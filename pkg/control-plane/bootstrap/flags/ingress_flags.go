package flags

import (
	"github.com/solo-io/gloo/pkg/control-plane/bootstrap"
	"github.com/spf13/cobra"
)

func AddIngressFlags(cmd *cobra.Command, opts *bootstrap.Options) {
	// TODO ingress.bind-adress
	cmd.PersistentFlags().StringVar(&opts.IngressOptions.BindAddress, "envoy.bind-adress", "::", "The address that the ingress envoy should bind to.")
	cmd.PersistentFlags().Uint32Var(&opts.IngressOptions.Port, "envoy.port", 8080, "The HTTP port envoy uses.")
	cmd.PersistentFlags().Uint32Var(&opts.IngressOptions.SecurePort, "envoy.secure-port", 8443, "The HTTPS port envoy uses.")
}
