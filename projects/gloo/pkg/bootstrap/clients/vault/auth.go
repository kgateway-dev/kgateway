package vault

import (
	"context"
	"time"

	errors "github.com/rotisserie/eris"
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
//go:generate mockgen -destination mocks/mock_auth.go -package mocks github.com/hashicorp/vault/api AuthMethod

type ClientAuth interface {
	// vault.AuthMethod provides Login(ctx context.Context, client *Client) (*Secret, error)
	vault.AuthMethod
	// ManageTokenRenewal should be called after a successful login to start the renewal process
	// This method may have many different types of implementation, from just a noop to spinning up a separate go routine
	ManageTokenRenewal(ctx context.Context, client *vault.Client, secret *vault.Secret)
}

var _ ClientAuth = &StaticTokenAuth{}
var _ ClientAuth = &RemoteTokenAuth{}

var (
	ErrEmptyToken          = errors.New("unable to authenticate to vault with empty token")
	ErrNoAuthInfo          = errors.New("no auth info was returned after login")
	ErrVaultAuthentication = func(err error) error {
		return errors.Wrap(err, "unable to authenticate to vault")
	}
	ErrPartialCredentials = func(err error) error {
		return errors.Wrap(err, "only partial credentials were provided for vault authorization with AWS IAM auth: ")
	}
	ErrAccessKeyId       = errors.New("access key id must be defined for vault authorization with AWS IAM auth")
	ErrSecretAccessKey   = errors.New("secret access key must be defined for vault authorization with AWS IAM auth")
	ErrInitializeWatcher = func(err error) error {
		return errors.Wrap(err, "unable to initialize new lifetime watcher for renewing auth token.")
	}
)

// ClientAuthFactory returns a vault ClientAuth based on the provided settings.
func ClientAuthFactory(vaultSettings *v1.Settings_VaultSecrets) (ClientAuth, error) {
	switch authMethod := vaultSettings.GetAuthMethod().(type) {
	case *v1.Settings_VaultSecrets_AccessToken:
		return NewStaticTokenAuth(authMethod.AccessToken), nil

	case *v1.Settings_VaultSecrets_Aws:
		awsAuth, err := newAwsAuthMethod(authMethod.Aws)
		if err != nil {
			return nil, err
		}

		tokenRenewer := NewVaultTokenRenewer(&NewVaultTokenRenewerParams{
			LeaseIncrement: int(authMethod.Aws.GetLeaseIncrement()),
			GetWatcher:     vaultGetWatcher,
		})

		return NewRemoteTokenAuth(awsAuth, tokenRenewer), nil

	default:
		// AuthMethod is the preferred API to define the policy for authenticating to vault
		// If one is not defined, we fall back to the deprecated API
		return NewStaticTokenAuth(vaultSettings.GetToken()), nil
	}
}

// NewStaticTokenAuth is a constructor for StaticTokenAuth
func NewStaticTokenAuth(token string) ClientAuth {
	return &StaticTokenAuth{
		token: token,
	}
}

type StaticTokenAuth struct {
	token string
}

// GetToken returns the value of the token field
func (s *StaticTokenAuth) GetToken() string {
	return s.token
}

// ManageTokenRenewal for StaticTokenAuth is a no-op
func (*StaticTokenAuth) ManageTokenRenewal(ctx context.Context, client *vault.Client, secret *vault.Secret) {
}

// Login logs in to vault using a static token
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

// NewRemoteTokenAuth is a constructor for RemoteTokenAuth
func NewRemoteTokenAuth(authMethod vault.AuthMethod, t TokenRenewer, retryOptions ...retry.Option) ClientAuth {

	// Standard retry options, which can be overridden by the loginRetryOptions parameter
	defaultRetryOptions := []retry.Option{
		retry.Delay(1 * time.Second),
		retry.DelayType(retry.BackOffDelay),
		retry.Attempts(3),
		retry.LastErrorOnly(true),
	}

	loginRetryOptions := append(defaultRetryOptions, retryOptions...)

	return &RemoteTokenAuth{
		authMethod:        authMethod,
		loginRetryOptions: loginRetryOptions,
		tokenRenewer:      t,
	}
}

type RemoteTokenAuth struct {
	authMethod        vault.AuthMethod
	tokenRenewer      TokenRenewer
	loginRetryOptions []retry.Option
}

func (r *RemoteTokenAuth) ManageTokenRenewal(ctx context.Context, client *vault.Client, secret *vault.Secret) {
	r.tokenRenewer.ManageTokenRenewal(ctx, client, r, secret)
}

// Login wraps the low-level login with retry logic
func (r *RemoteTokenAuth) Login(ctx context.Context, client *vault.Client) (*vault.Secret, error) {
	var (
		loginResponse *vault.Secret
		loginErr      error
	)

	// Set the "retryIf" option here. We don't want this to be overridden, and the context isn't
	// available in the contructor to configure this
	retryOptions := append(
		r.loginRetryOptions,
		retry.RetryIf(func(err error) bool {
			// if the parent context is cancelled,
			// stop retrying.
			select {
			case <-ctx.Done():
				return false
			default:
				return true
			}
		}),
	)

	loginErr = retry.Do(func() error {
		loginResponse, loginErr = r.loginOnce(ctx, client)
		return loginErr
	}, retryOptions...)

	// As noted above, we need to check the context here, because our retry function can not return errors
	if ctx.Err() != nil {
		return nil, errors.Wrap(ctx.Err(), "login canceled")
	}

	if loginErr != nil {
		return nil, loginErr
	}

	return loginResponse, nil
}

func (r *RemoteTokenAuth) loginOnce(ctx context.Context, client *vault.Client) (*vault.Secret, error) {
	loginResponse, loginErr := r.authMethod.Login(ctx, client)
	if loginErr != nil {
		contextutils.LoggerFrom(ctx).Errorf("unable to authenticate to vault: %v", loginErr)
		utils.Measure(ctx, MLastLoginFailure, time.Now().Unix())
		utils.MeasureOne(ctx, MLoginFailures)
		return nil, ErrVaultAuthentication(loginErr)
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

func newAwsAuthMethod(aws *v1.Settings_VaultAwsAuth) (*awsauth.AWSAuth, error) {
	// The AccessKeyID and SecretAccessKey are not required in the case of using temporary credentials from assumed roles with AWS STS or IRSA.
	// STS: https://docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_temp_use-resources.html
	// IRSA: https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html
	var possibleErrors []error
	if accessKeyId := aws.GetAccessKeyId(); accessKeyId != "" {
		awsutils.SetAccessKeyEnv(accessKeyId)
	} else {
		possibleErrors = append(possibleErrors, ErrAccessKeyId)
	}

	if secretAccessKey := aws.GetSecretAccessKey(); secretAccessKey != "" {
		awsutils.SetSecretAccessKeyEnv(secretAccessKey)
	} else {
		possibleErrors = append(possibleErrors, ErrSecretAccessKey)
	}

	// if we have only partial configuration set
	if len(possibleErrors) == 1 {
		return nil, ErrPartialCredentials(possibleErrors[0])
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
