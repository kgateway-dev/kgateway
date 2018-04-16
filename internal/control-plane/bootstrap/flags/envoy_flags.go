package flags

import (
	"github.com/solo-io/gloo/internal/control-plane/bootstrap"
	"github.com/spf13/cobra"
)

func AddEnvoyFlags(cmd *cobra.Command, opts *bootstrap.Options) {
	// TODO ingress.bind-adress
	cmd.PersistentFlags().StringVar(&opts.EnvoyOptions.BindAddress, "envoy.bind-adress", "", "The address that the ingress envoy should bind to.")
}
