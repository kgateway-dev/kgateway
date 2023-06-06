package bootstrap

import (
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"

	"github.com/hashicorp/vault/api"
	_ "github.com/hashicorp/vault/api/auth/aws"
)

// Deprecated. Use bootstrap/clients
const DefaultPathPrefix = clients.DefaultPathPrefix

// Deprecated. Use bootstrap/clients
func NewVaultSecretClientFactory(client *api.Client, pathPrefix, rootKey string) factory.ResourceClientFactory {
	f := func() *api.Client { return client }
	return clients.NewVaultSecretClientFactory(f, pathPrefix, rootKey)
}

// Deprecated. Use bootstrap/clients
func VaultClientForSettings(vaultSettings *v1.Settings_VaultSecrets) (*api.Client, error) {
	return clients.VaultClientForSettings(vaultSettings)
}
