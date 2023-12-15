package clients

import (
	"context"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients/vault"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"

	"github.com/hashicorp/vault/api"
	_ "github.com/hashicorp/vault/api/auth/aws"
)

// The DefaultPathPrefix may be overridden to allow for non-standard vault mount paths
const DefaultPathPrefix = "secret"

type VaultClientInitFunc func() *api.Client

// NoopVaultClientInitFunc returns the provided client without any modification
func NoopVaultClientInitFunc(c *api.Client) VaultClientInitFunc {
	return func() *api.Client {
		return c
	}
}

// NewVaultSecretClientFactory consumes a vault client along with a set of basic configurations for retrieving info with the client
func NewVaultSecretClientFactory(clientInit VaultClientInitFunc, pathPrefix, rootKey string) factory.ResourceClientFactory {
	return &factory.VaultSecretClientFactory{
		Vault:      clientInit(),
		RootKey:    rootKey,
		PathPrefix: pathPrefix,
	}
}

// VaultClientForSettings returns a vault client based on the provided settings.
func VaultClientForSettings(ctx context.Context, vaultSettings *v1.Settings_VaultSecrets, vaultAuth vault.ClientAuth) (*api.Client, error) {
	return vault.NewAuthenticatedClient(ctx, vaultSettings, vaultAuth)
}
