package bootstrap

import (
	"github.com/hashicorp/vault/api"
	_ "github.com/hashicorp/vault/api/auth/aws"
	errors "github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// The DefaultPathPrefix may be overridden to allow for non-standard vault mount paths
const DefaultPathPrefix = "secret"

// NewVaultSecretClientFactory consumes a vault client along with a set of basic configurations for retrieving info with the client
func NewVaultSecretClientFactory(client *api.Client, pathPrefix, rootKey string) factory.ResourceClientFactory {
	return &factory.VaultSecretClientFactory{
		Vault:      client,
		RootKey:    rootKey,
		PathPrefix: pathPrefix,
	}
}

func VaultClientForSettings(vaultSettings *v1.Settings_VaultSecrets) (*api.Client, error) {
	cfg, err := parseVaultSettings(vaultSettings)
	client, err := api.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	client, err = configureVaultAuth(vaultSettings, client)
	token := vaultSettings.GetToken()
	if token == "" {
		return nil, errors.Errorf("token is required for connecting to vault")
	}
	client.SetToken(token)

	return client, nil
}

func configureVaultAuth(vaultSettings *v1.Settings_VaultSecrets, client *api.Client) (*api.Client, error) {
	// switch tlsCfg := vaultSettings.GetTlsConfig().(type) {
	// default: return nil, nil
	// }
	return nil, nil
}

func parseVaultSettings(vaultSettings *v1.Settings_VaultSecrets) (*api.Config, error) {
	cfg := api.DefaultConfig()

	if addr := vaultSettings.GetAddress(); addr != "" {
		cfg.Address = addr
	}
	if tlsConfig := parseTlsSettings(vaultSettings); tlsConfig != nil {
		if err := cfg.ConfigureTLS(tlsConfig); err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

func parseTlsSettings(vaultSettings *v1.Settings_VaultSecrets) *api.TLSConfig {
	var tlsConfig *api.TLSConfig
	addStringSetting := func(s string, addSettingFunc func(string)) {
		if tlsConfig == nil {
			tlsConfig = &api.TLSConfig{}
		}
		if s != "" {
			addSettingFunc(s)
		}
	}
	addBoolSetting := func(b *wrapperspb.BoolValue, addSettingFunc func(bool)) {
		if tlsConfig == nil {
			tlsConfig = &api.TLSConfig{}
		}
		if b != nil {
			addSettingFunc(b.GetValue())
		}
	}
	addStringSetting(vaultSettings.GetCaCert(), func(s string) { tlsConfig.CACert = s })
	addStringSetting(vaultSettings.GetCaPath(), func(s string) { tlsConfig.CAPath = s })
	addStringSetting(vaultSettings.GetClientCert(), func(s string) { tlsConfig.ClientCert = s })
	addStringSetting(vaultSettings.GetClientKey(), func(s string) { tlsConfig.ClientKey = s })
	addStringSetting(vaultSettings.GetTlsServerName(), func(s string) { tlsConfig.TLSServerName = s })
	addBoolSetting(vaultSettings.GetInsecure(), func(b bool) { tlsConfig.Insecure = b })

	return tlsConfig

}
