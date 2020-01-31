package bootstrap

import (
	"github.com/hashicorp/vault/api"
	errors "github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

func VaultClientForSettings(vaultSettings *v1.Settings_VaultSecrets) (*api.Client, error) {
	cfg := api.DefaultConfig()

	var tlsCfg *api.TLSConfig
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

	client, err := api.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	if tlsCfg != nil {
		if err := cfg.ConfigureTLS(tlsCfg); err != nil {
			return nil, err
		}
	}
	token := vaultSettings.GetToken()
	if token == "" {
		return nil, errors.Errorf("token is required for connecting to vault")
	}
	client.SetToken(token)

	return client, nil
}
