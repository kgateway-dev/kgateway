package clients

import (
	"context"

	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients/vault"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"

	"github.com/hashicorp/vault/api"
	_ "github.com/hashicorp/vault/api/auth/aws"
)

// The DefaultPathPrefix may be overridden to allow for non-standard vault mount paths
const DefaultPathPrefix = "secret"

type VaultClientInitFunc func(ctx context.Context) *api.Client

func NoopVaultClientInitFunc(c *api.Client) VaultClientInitFunc {
	return func(_ context.Context) *api.Client {
		return c
	}
}

// NewVaultSecretClientFactory consumes a vault client along with a set of basic configurations for retrieving info with the client
func NewVaultSecretClientFactory(ctx context.Context, clientInit VaultClientInitFunc, pathPrefix, rootKey string) factory.ResourceClientFactory {
	return &factory.VaultSecretClientFactory{
		Vault:      clientInit(ctx),
		RootKey:    rootKey,
		PathPrefix: pathPrefix,
	}
}

// VaultClientForSettings returns a vault client based on the provided settings.
func VaultClientForSettings(ctx context.Context, vaultSettings *v1.Settings_VaultSecrets) (*api.Client, error) {
	vaultAuth, err := vault.ClientAuthFactory(vaultSettings)
	if err != nil {
		err = eris.Wrap(err, "check Settings configuration")
		contextutils.LoggerFrom(ctx).Error(err)
	}
	return vault.NewAuthenticatedClient(ctx, vaultSettings, vaultAuth)
}
