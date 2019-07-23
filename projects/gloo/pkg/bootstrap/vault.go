package bootstrap

import (
	"github.com/hashicorp/vault/api"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

func VaultClientForSettings(settings *v1.Settings) (*api.Client, error) {
	cfg := api.DefaultConfig()

	vaultSettings := settings.GetVaultSecretSource()
	var tlsCfg *api.TLSConfig
	if vaultSettings != nil {
		if addr := vaultSettings.GetAddress(); addr != "" {
			cfg.Address = addr
		}
		if caCert := vaultSettings.GetCaCert(); caCert != "" {
			tlsCfg = &api.TLSConfig{
				CACert: caCert,
			}
		}
		if caPath := vaultSettings.GetCaPath(); caPath != "" {
			if tlsCfg == nil {
				tlsCfg = &api.TLSConfig{}
			}
			tlsCfg.CAPath = caPath
		}
		if clientCert := vaultSettings.GetClientCert(); clientCert != "" {
			if tlsCfg == nil {
				tlsCfg = &api.TLSConfig{}
			}
			tlsCfg.ClientCert = clientCert
		}
		if clientKey := vaultSettings.GetClientKey(); clientKey != "" {
			if tlsCfg == nil {
				tlsCfg = &api.TLSConfig{}
			}
			tlsCfg.ClientKey = clientKey
		}
		if tlsServerName := vaultSettings.GetTlsServerName(); tlsServerName != "" {
			if tlsCfg == nil {
				tlsCfg = &api.TLSConfig{}
			}
			tlsCfg.TLSServerName = tlsServerName
		}
		if insecure := vaultSettings.GetInsecure(); insecure != nil {
			if tlsCfg == nil {
				tlsCfg = &api.TLSConfig{}
			}
			tlsCfg.Insecure = insecure.GetValue()
		}
	}

	return api.NewClient(cfg)
}
