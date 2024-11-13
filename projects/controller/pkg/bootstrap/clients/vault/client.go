package vault

import (
	"context"

	vault "github.com/hashicorp/vault/api"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// NewAuthenticatedClient returns a vault client that has been authenticated with the provided settings,
// or an error if construction or authentication fails.
func NewAuthenticatedClient(ctx context.Context, vaultSettings *v1.Settings_VaultSecrets, clientAuth ClientAuth) (*vault.Client, error) {
	client, err := NewUnauthenticatedClient(vaultSettings)
	if err != nil {
		return nil, err
	}

	secret, err := AuthenticateClient(ctx, client, clientAuth)
	if err != nil {
		return nil, err
	}

	clientAuth.ManageTokenRenewal(ctx, client, secret)

	return client, nil
}

// NewUnauthenticatedClient returns a vault client that has not yet been authenticated
func NewUnauthenticatedClient(vaultSettings *v1.Settings_VaultSecrets) (*vault.Client, error) {
	cfg, err := parseVaultSettings(vaultSettings)
	if err != nil {
		return nil, err
	}

	return vault.NewClient(cfg)
}

// AuthenticateClient authenticates the provided vault client with the provided clientAuth.
func AuthenticateClient(ctx context.Context, client *vault.Client, clientAuth ClientAuth) (*vault.Secret, error) {
	secret, err := client.Auth().Login(ctx, clientAuth)
	if err != nil {
		return nil, err
	}

	return secret, nil
}

func parseVaultSettings(vaultSettings *v1.Settings_VaultSecrets) (*vault.Config, error) {
	cfg := vault.DefaultConfig()

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

func parseTlsSettings(vaultSettings *v1.Settings_VaultSecrets) *vault.TLSConfig {
	var tlsConfig *vault.TLSConfig

	// helper functions to avoid repeated nilchecking
	addStringSetting := func(s string, addSettingFunc func(string)) {
		if s == "" {
			return
		}
		if tlsConfig == nil {
			tlsConfig = &vault.TLSConfig{}
		}
		addSettingFunc(s)
	}
	addBoolSetting := func(b *wrapperspb.BoolValue, addSettingFunc func(bool)) {
		if b == nil {
			return
		}
		if tlsConfig == nil {
			tlsConfig = &vault.TLSConfig{}
		}
		addSettingFunc(b.GetValue())
	}

	setCaCert := func(s string) { tlsConfig.CACert = s }
	setCaPath := func(s string) { tlsConfig.CAPath = s }
	setClientCert := func(s string) { tlsConfig.ClientCert = s }
	setClientKey := func(s string) { tlsConfig.ClientKey = s }
	setTlsServerName := func(s string) { tlsConfig.TLSServerName = s }
	setInsecure := func(b bool) { tlsConfig.Insecure = b }

	// Add our settings to the vault TLS config, preferring settings set in the
	// new TlsConfig field if it is used to those in the deprecated fields
	if tlsSettings := vaultSettings.GetTlsConfig(); tlsSettings == nil {
		addStringSetting(vaultSettings.GetCaCert(), setCaCert)
		addStringSetting(vaultSettings.GetCaPath(), setCaPath)
		addStringSetting(vaultSettings.GetClientCert(), setClientCert)
		addStringSetting(vaultSettings.GetClientKey(), setClientKey)
		addStringSetting(vaultSettings.GetTlsServerName(), setTlsServerName)
		addBoolSetting(vaultSettings.GetInsecure(), setInsecure)
	} else {
		addStringSetting(tlsSettings.GetCaCert(), setCaCert)
		addStringSetting(tlsSettings.GetCaPath(), setCaPath)
		addStringSetting(tlsSettings.GetClientCert(), setClientCert)
		addStringSetting(tlsSettings.GetClientKey(), setClientKey)
		addStringSetting(tlsSettings.GetTlsServerName(), setTlsServerName)
		addBoolSetting(tlsSettings.GetInsecure(), setInsecure)
	}

	return tlsConfig

}
