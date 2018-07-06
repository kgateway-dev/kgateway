package flags

import (
	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/spf13/cobra"
)

func AddConsulFlags(cmd *cobra.Command, opts *bootstrap.Options) {
	cmd.PersistentFlags().StringVar(&opts.ConsulOptions.RootPath, "consul.root", "gloo", "prefix for all k/v pairs stored in consul by gloo, when using consul for storage")
	cmd.PersistentFlags().StringVar(&opts.ConsulOptions.Datacenter, "consul.datacenter", "", "datacenter of the consul server when using consul for storage or service discovery")
	cmd.PersistentFlags().StringVar(&opts.ConsulOptions.Address, "consul.address", "", "address (including port) of the consul server to connect to when using consul for storage or service discovery")
	cmd.PersistentFlags().StringVar(&opts.ConsulOptions.Scheme, "consul.scheme", "", "uri scheme for the consul server")
	cmd.PersistentFlags().StringVar(&opts.ConsulOptions.Token, "consul.token", "", "token is used to provide a per-request ACL token to override the default")
	cmd.PersistentFlags().StringVar(&opts.ConsulOptions.Username, "consul.username", "", "username for authenticating to the consul server, if using basic auth")
	cmd.PersistentFlags().StringVar(&opts.ConsulOptions.Password, "consul.password", "", "password for authenticating to the consul server, if using basic auth")
	cmd.PersistentFlags().BoolVar(&opts.ConsulOptions.Connect, "consul.connect", false, "should we do connect endpoint discovery?")
}
