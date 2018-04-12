package flags

import (
	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/spf13/cobra"
)

func AddVaultFlags(cmd *cobra.Command, opts *bootstrap.Options) {
	cmd.PersistentFlags().StringVar(&opts.VaultOptions.VaultAddr, "vault.addr", "", "url for vault server")
	cmd.PersistentFlags().StringVar(&opts.VaultOptions.VaultToken, "vault.token", "", "token for authenticating to vault")
	cmd.PersistentFlags().StringVar(&opts.VaultOptions.VaultTokenFile, "vault.tokenfile", "", "file containing token for authenticating to vault")
	cmd.PersistentFlags().IntVar(&opts.VaultOptions.Retries, "vault.retries", 3, "number of times to retry failed requests to vault")
	cmd.PersistentFlags().StringVar(&opts.VaultOptions.RootPath, "vault.rootpath", "gloo", "root vault directory for secret storage")
}
