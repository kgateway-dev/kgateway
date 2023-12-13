package vault

import (
	"context"
	"errors"
	"time"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/utils/awsutils"

	"github.com/avast/retry-go"
	vault "github.com/hashicorp/vault/api"
	awsauth "github.com/hashicorp/vault/api/auth/aws"
	"github.com/solo-io/gloo/pkg/utils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
)

// In an ideal world, we would re-use the mocks provided by an external library.
// Since the vault.AuthMethod interface does not have corresponding mocks, we have to define our own.
// todo - just mock the ClientAuth interface we define
//go:generate mockgen -destination mocks/mock_auth.go -package mocks github.com/hashicorp/vault/api AuthMethod

type ClientAuth interface {
	vault.AuthMethod
	// Start Renewal should be called after a successful login to start the renewal process
	// it starts a go routine that will renew the token at the appropriate time, and so it does
	// not return a value, it just goes off and does its thing
	// StartRenewal(ctx context.Context, client *vault.Client, secret *vault.Secret)
}

var _ ClientAuth = &StaticTokenAuth{}
var _ ClientAuth = &RemoteTokenAuth{}

var (
	ErrEmptyToken = errors.New("unable to authenticate to vault with empty token. check Settings configuration")
	ErrNoAuthInfo = errors.New("no auth info was returned after login")
)

// ClientAuthFactory returns a vault ClientAuth based on the provided settings.
func ClientAuthFactory(vaultSettings *v1.Settings_VaultSecrets) (ClientAuth, error) {
	switch tlsCfg := vaultSettings.GetAuthMethod().(type) {
	case *v1.Settings_VaultSecrets_AccessToken:
		return NewStaticTokenAuth(tlsCfg.AccessToken), nil

	case *v1.Settings_VaultSecrets_Aws:
		awsAuth, err := newAwsAuthMethod(tlsCfg.Aws)
		if err != nil {
			return nil, err
		}

		return NewRemoteTokenAuth(awsAuth), nil

	default:
		// AuthMethod is the preferred API to define the policy for authenticating to vault
		// If one is not defined, we fall back to the deprecated API
		return NewStaticTokenAuth(vaultSettings.GetToken()), nil
	}
}

func NewStaticTokenAuth(token string) ClientAuth {
	return &StaticTokenAuth{
		token: token,
	}
}

type StaticTokenAuth struct {
	token string
}

func (s *StaticTokenAuth) GetToken() string {
	return s.token
}

// func (s *StaticTokenAuth) StartRenewal(_ context.Context, client *vault.Client, _ *vault.Secret) {
// 	// static tokens do not support renewal
// }

func (s *StaticTokenAuth) Login(ctx context.Context, _ *vault.Client) (*vault.Secret, error) {
	if s.GetToken() == "" {
		utils.Measure(ctx, MLastLoginFailure, time.Now().Unix())
		utils.MeasureOne(ctx, MLoginFailures)
		return nil, ErrEmptyToken
	}

	contextutils.LoggerFrom(ctx).Debug("successfully authenticated to vault with static token")
	utils.Measure(ctx, MLastLoginSuccess, time.Now().Unix())
	utils.MeasureOne(ctx, MLoginSuccesses)
	return &vault.Secret{
		Auth: &vault.SecretAuth{
			ClientToken: s.token,
		},
	}, nil
}

func NewRemoteTokenAuth(authMethod vault.AuthMethod, retryOptions ...retry.Option) ClientAuth {
	// Standard retry options, which can be overridden by the loginRetryOptions parameter
	defaultRetryOptions := []retry.Option{
		retry.Delay(1 * time.Second),
		retry.DelayType(retry.BackOffDelay),
		retry.Attempts(1),
		retry.LastErrorOnly(true),
	}
	loginRetryOptions := append(defaultRetryOptions, retryOptions...)

	return &RemoteTokenAuth{
		authMethod:        authMethod,
		loginRetryOptions: loginRetryOptions,
	}
}

type RemoteTokenAuth struct {
	authMethod        vault.AuthMethod
	loginRetryOptions []retry.Option
}

func (r *RemoteTokenAuth) Login(ctx context.Context, client *vault.Client) (*vault.Secret, error) {
	var (
		loginResponse *vault.Secret
		loginErr      error
	)

	loginErr = retry.Do(func() error {
		// If the context is cancelled, we should not retry, but we also can't return an error or we will retry
		// so we return nil and rely on the caller to check the context
		if ctx.Err() != nil {
			return nil
		}
		loginResponse, loginErr = r.loginOnce(ctx, client)
		return loginErr
	}, r.loginRetryOptions...)

	// As noted above, we need to check the context here, because our retry function can not return errors
	if ctx.Err() != nil {
		return nil, eris.Wrap(ctx.Err(), "Login canceled")
	}

	return loginResponse, loginErr
}

func (r *RemoteTokenAuth) loginOnce(ctx context.Context, client *vault.Client) (*vault.Secret, error) {
	loginResponse, loginErr := r.authMethod.Login(ctx, client)
	if loginErr != nil {
		contextutils.LoggerFrom(ctx).Errorf("unable to authenticate to vault: %v", loginErr)
		utils.Measure(ctx, MLastLoginFailure, time.Now().Unix())
		utils.MeasureOne(ctx, MLoginFailures)
		return nil, eris.Wrapf(loginErr, "unable to authenticate to vault")
	}

	if loginResponse == nil {
		contextutils.LoggerFrom(ctx).Error(ErrNoAuthInfo)
		utils.Measure(ctx, MLastLoginFailure, time.Now().Unix())
		utils.MeasureOne(ctx, MLoginFailures)
		return nil, ErrNoAuthInfo
	}

	contextutils.LoggerFrom(ctx).Debugf("successfully authenticated to vault %v", loginResponse)
	utils.Measure(ctx, MLastLoginSuccess, time.Now().Unix())
	utils.MeasureOne(ctx, MLoginSuccesses)
	return loginResponse, nil
}

// func (r *RemoteTokenAuth) StartRenewal(ctx context.Context, client *vault.Client, secret *vault.Secret) {
// 	// todo - implement renewal
// }

func newAwsAuthMethod(aws *v1.Settings_VaultAwsAuth) (*awsauth.AWSAuth, error) {
	// The AccessKeyID and SecretAccessKey are not required in the case of using temporary credentials from assumed roles with AWS STS or IRSA.
	// STS: https://docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_temp_use-resources.html
	// IRSA: https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html
	var possibleErrStrings []string
	if accessKeyId := aws.GetAccessKeyId(); accessKeyId != "" {
		awsutils.SetAccessKeyEnv(accessKeyId)
	} else {
		possibleErrStrings = append(possibleErrStrings, "access key id must be defined for AWS IAM auth")
	}

	if secretAccessKey := aws.GetSecretAccessKey(); secretAccessKey != "" {
		awsutils.SetSecretAccessKeyEnv(secretAccessKey)
	} else {
		possibleErrStrings = append(possibleErrStrings, "secret access key must be defined for AWS IAM auth")
	}

	// if we have only partial configuration set
	if len(possibleErrStrings) == 1 {
		return nil, errors.New("only partial credentials were provided for AWS IAM auth: " + possibleErrStrings[0])
	}

	// At this point, we either have full auth configuration set, or are in an ec2 environment, where vault will infer the credentials.
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
		awsutils.SetSessionTokenEnv(sessionToken)
	}

	return awsauth.NewAWSAuth(loginOptions...)
}
