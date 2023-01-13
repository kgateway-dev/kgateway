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
	return configureVaultAuth(vaultSettings, client)
}

func configureVaultAuth(vaultSettings *v1.Settings_VaultSecrets, client *api.Client) (*api.Client, error) {
	switch tlsCfg := vaultSettings.GetAuthMethod().(type) {
	case *v1.Settings_VaultSecrets_AccessToken:
		client.SetToken(tlsCfg.AccessToken)
	case *v1.Settings_VaultSecrets_Aws:
		configureAwsAuth(tlsCfg.Aws, client)
	default:
		token := vaultSettings.GetToken()
		if token == "" {
			return nil, errors.Errorf("unable to determine vault authentication method. check Settings configuration")
		}
		client.SetToken(token)
	}
	return client, nil
}

func configureAwsAuth(aws *v1.Settings_VaultAwsAuth, client *api.Client) {

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

	// helper functions to avoid repeated nilchecking
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

	// Add our settings to the vault TLS config, preferring settings set in the
	// new TlsConfig field to those in the deprecated fields
	setCaCert := func(s string) { tlsConfig.CACert = s }
	addStringSetting(vaultSettings.GetCaCert(), setCaCert)
	addStringSetting(vaultSettings.GetTlsConfig().GetCaCert(), setCaCert)

	setCaPath := func(s string) { tlsConfig.CAPath = s }
	addStringSetting(vaultSettings.GetCaPath(), setCaPath)
	addStringSetting(vaultSettings.GetTlsConfig().GetCaPath(), setCaPath)

	setClientCert := func(s string) { tlsConfig.ClientCert = s }
	addStringSetting(vaultSettings.GetClientCert(), setClientCert)
	addStringSetting(vaultSettings.GetTlsConfig().GetClientCert(), setClientCert)

	setClientKey := func(s string) { tlsConfig.ClientKey = s }
	addStringSetting(vaultSettings.GetClientKey(), setClientKey)
	addStringSetting(vaultSettings.GetTlsConfig().GetClientKey(), setClientKey)

	setTlsServerName := func(s string) { tlsConfig.TLSServerName = s }
	addStringSetting(vaultSettings.GetTlsServerName(), setTlsServerName)
	addStringSetting(vaultSettings.GetTlsConfig().GetTlsServerName(), setTlsServerName)

	setInsecure := func(b bool) { tlsConfig.Insecure = b }
	addBoolSetting(vaultSettings.GetInsecure(), setInsecure)
	addBoolSetting(vaultSettings.GetTlsConfig().GetInsecure(), setInsecure)

	return tlsConfig

}
