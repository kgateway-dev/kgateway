package clients

import (
	"context"
	"os"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"

	"github.com/hashicorp/vault/api"
	_ "github.com/hashicorp/vault/api/auth/aws"
	awsauth "github.com/hashicorp/vault/api/auth/aws"
	errors "github.com/rotisserie/eris"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type vaultSecretClientSettings struct {
	vault *api.Client

	// Vault's path where resources are located.
	root string

	// Tells Vault which secrets engine it should route traffic to. Defaults to "secret".
	// https://learn.hashicorp.com/tutorials/vault/getting-started-secrets-engines
	pathPrefix string
}

// The DefaultPathPrefix may be overridden to allow for non-standard vault mount paths
const DefaultPathPrefix = "secret"

type VaultClientInitFunc func() *api.Client

func NoopVaultClientInitFunc(c *api.Client) VaultClientInitFunc {
	return func() *api.Client {
		return c
	}
}

var (
	ErrNilVaultClient = errors.New("vault API client failed to initialize")
)

// NewVaultSecretClientFactory consumes a vault client along with a set of basic configurations for retrieving info with the client
func NewVaultSecretClientFactory(clientInit VaultClientInitFunc, pathPrefix, rootKey string) factory.ResourceClientFactory {
	return &factory.VaultSecretClientFactory{
		Vault:      clientInit(),
		RootKey:    rootKey,
		PathPrefix: pathPrefix,
	}
}

func VaultClientForSettings(vaultSettings *v1.Settings_VaultSecrets) (*api.Client, error) {
	cfg, err := parseVaultSettings(vaultSettings)
	if err != nil {
		return nil, err
	}
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
		if s == "" {
			return
		}
		if tlsConfig == nil {
			tlsConfig = &api.TLSConfig{}
		}
		addSettingFunc(s)
	}
	addBoolSetting := func(b *wrapperspb.BoolValue, addSettingFunc func(bool)) {
		if b == nil {
			return
		}
		if tlsConfig == nil {
			tlsConfig = &api.TLSConfig{}
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
		addStringSetting(vaultSettings.GetTlsConfig().GetCaCert(), setCaCert)
		addStringSetting(vaultSettings.GetTlsConfig().GetCaPath(), setCaPath)
		addStringSetting(vaultSettings.GetTlsConfig().GetClientCert(), setClientCert)
		addStringSetting(vaultSettings.GetTlsConfig().GetClientKey(), setClientKey)
		addStringSetting(vaultSettings.GetTlsConfig().GetTlsServerName(), setTlsServerName)
		addBoolSetting(vaultSettings.GetTlsConfig().GetInsecure(), setInsecure)
	}

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

// This indirection function exists to more easily enable further extenstion of AWS auth
// to support EC2 auth method in the future
func configureAwsAuth(aws *v1.Settings_VaultAwsAuth, client *api.Client) (*api.Client, error) {
	return configureAwsIamAuth(aws, client)
}

func configureAwsIamAuth(aws *v1.Settings_VaultAwsAuth, client *api.Client) (*api.Client, error) {
	// The AccessKeyID and SecretAccessKey are not required in the case of using temporary credentials from assumed roles with AWS STS or IRSA.
	// STS: https://docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_temp_use-resources.html
	// IRSA: https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html
	if accessKeyId := aws.GetAccessKeyId(); accessKeyId != "" {
		os.Setenv("AWS_ACCESS_KEY_ID", accessKeyId)
	}

	if secretAccessKey := aws.GetSecretAccessKey(); secretAccessKey != "" {
		os.Setenv("AWS_SECRET_ACCESS_KEY", secretAccessKey)
	}

	loginOptions := []awsauth.LoginOption{awsauth.WithIAMAuth()}

	if role := aws.GetVaultRole(); role != "" {
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

	if sessionToken := aws.GetSessionToken(); sessionToken != "" {
		os.Setenv("AWS_SESSION_TOKEN", sessionToken)
	}

	awsAuth, err := awsauth.NewAWSAuth(loginOptions...)
	if err != nil {
		return nil, err
	}

	// TODO(jbohanon) set up auth token refreshing with client.NewLifetimeWatcher()
	authInfo, err := client.Auth().Login(context.Background(), awsAuth)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to login to AWS auth method")
	}
	if authInfo == nil {
		return nil, errors.New("no auth info was returned after login")
	}

	return client, nil
}
