package flags

import (
	"github.com/solo-io/gloo/internal/upstream-discovery/bootstrap"
	"github.com/spf13/cobra"
)

func AddUpstreamDiscoveryFlags(cmd *cobra.Command, opts *bootstrap.Options) {
	cmd.PersistentFlags().BoolVar(&opts.UpstreamDiscoveryOptions.EnableDiscoveryForKubernetes, "enable.kubernetes",
		false, "Enable upstream discovery for Kubernetes. Note, if all upstream discovery is disabled, "+
			"kubernetes discovery will be activated automatically")
	cmd.PersistentFlags().BoolVar(&opts.UpstreamDiscoveryOptions.EnableDiscoveryForCopilot, "enable.cloudfoundry",
		false, "Enable upstream discovery for CloudFoundry via Copilot")
}
