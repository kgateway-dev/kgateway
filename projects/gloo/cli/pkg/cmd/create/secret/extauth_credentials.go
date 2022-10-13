package secret

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/spf13/cobra"
)

func ExtAuthAccountCredentialsCmd(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "authcredentials",
		Short: `Create an AuthenticationCredentials secret with the given name (Enterprise)`,
		Long: "Create an AuthenticationCredentials secret with the given name. The AuthenticationCredentials secret contains " +
			"a username and password to bind as an LDAP service account. This is an enterprise-only feature.",
		RunE: func(c *cobra.Command, args []string) error {}
	}
	return cmd
}

func persistCredentialsSecret() {

}