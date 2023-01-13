package bootstrap

import (
	"context"
	"os"

	"github.com/hashicorp/vault/api"
	_ "github.com/hashicorp/vault/api/auth/aws"
	awsauth "github.com/hashicorp/vault/api/auth/aws"
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

func configureVaultAuth(vaultSettings *v1.Settings_VaultSecrets, client *api.Client) (*api.Client, error) {
	// each case returns
	switch tlsCfg := vaultSettings.GetAuthMethod().(type) {
	case *v1.Settings_VaultSecrets_AccessToken:
		client.SetToken(tlsCfg.AccessToken)
		return client, nil
	case *v1.Settings_VaultSecrets_Aws:
		return configureAwsAuth(tlsCfg.Aws, client)
	default:
		// We don't have one of the defined auth methods, so try to fall back to the
		// deprecated token field before erroring
		token := vaultSettings.GetToken()
		if token == "" {
			return nil, errors.Errorf("unable to determine vault authentication method. check Settings configuration")
		}
		client.SetToken(token)
		return client, nil
	}
}

func configureAwsAuth(aws *v1.Settings_VaultAwsAuth, client *api.Client) (*api.Client, error) {
	return configureAwsIamAuth(aws, client)
}

func configureAwsIamAuth(aws *v1.Settings_VaultAwsAuth, client *api.Client) (*api.Client, error) {

	if accessKeyId := aws.GetAccessKeyId(); accessKeyId == "" {
		return nil, errors.New("access key id must be defined for AWS IAM auth")
	} else {
		os.Setenv("AWS_ACCESS_KEY_ID", accessKeyId)
	}

	// TODO(jbohanon) change accessor once we figure out API for this secret value
	// this is a secret value and should not exist in the Settings in plaintext
	if secretAccessKey := aws.GetSecretAccessKey(); secretAccessKey == "" {
		return nil, errors.New("secret access key must be defined for AWS IAM auth")
	} else {
		os.Setenv("AWS_SECRET_ACCESS_KEY", secretAccessKey)
	}

	loginOptions := []awsauth.LoginOption{awsauth.WithIAMAuth()}

	if role := aws.GetRole(); role != "" {
		loginOptions = append(loginOptions, awsauth.WithRole(role))
	}

	if region := aws.GetRegion(); region != "" {
		loginOptions = append(loginOptions, awsauth.WithRegion(region))
	}

	if iamServerIdHeader := aws.GetIamServerIdHeader(); iamServerIdHeader != "" {
		loginOptions = append(loginOptions, awsauth.WithIAMServerIDHeader(iamServerIdHeader))
	}

	if mountPath := aws.GetMountPath(); mountPath != "" {
		loginOptions = append(loginOptions, awsauth.WithMountPath(mountPath))
	}

	// TODO(jbohanon) change accessor once we figure out API for this secret value
	// this is a secret value and should not exist in the Settings in plaintext
	if sessionToken := aws.GetSessionToken(); sessionToken != "" {
		os.Setenv("AWS_SESSION_TOKEN", sessionToken)
	}

	awsAuth, err := awsauth.NewAWSAuth(loginOptions...)
	if err != nil {
		return nil, err
	}

	authInfo, err := client.Auth().Login(context.TODO(), awsAuth)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to login to AWS auth method")
	}
	if authInfo == nil {
		return nil, errors.New("no auth info was returned after login")
	}

	return client, nil
}
