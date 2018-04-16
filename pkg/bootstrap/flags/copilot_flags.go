package flags

import (
	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/spf13/cobra"
)

func AddCoPilotFlags(cmd *cobra.Command, opts *bootstrap.Options) {
	cmd.PersistentFlags().StringVar(&opts.CoPilotOptions.Address, "copilot.address", "", "ip:port of copilot server")
	cmd.PersistentFlags().StringVar(&opts.CoPilotOptions.ServerCA, "copilot.server-ca", "", "path to cert for the copilot server CA")
	cmd.PersistentFlags().StringVar(&opts.CoPilotOptions.ClientCert, "copilot.client-cert", "", "path to cert for the copilot client")
	cmd.PersistentFlags().StringVar(&opts.CoPilotOptions.ClientKey, "copilot.client-key", "", "path to key for the copilot client")
}
