package vault

import (
	"context"
	"math/rand"
	"time"

	vault "github.com/hashicorp/vault/api"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/utils"
	"github.com/solo-io/go-utils/contextutils"
)

var (
	ErrAuthNotDefined = errors.New("auth method not defined")
)

// TokenWatcher is an interface that wraps the DoneCh, RenewCh, Start, and Stop functions of the vault LifetimeWatcher
type TokenWatcher interface {
	DoneCh() <-chan error
	RenewCh() <-chan *vault.RenewOutput
	Stop()
	Start()
}

// TokenRenewer is an interface that wraps the ManageTokenRenewal method. This lets us inject a noop function when we want
// to disable token renewal/the goroutine for testing
type TokenRenewer interface {
	ManageTokenRenewal(ctx context.Context, client *vault.Client, clientAuth ClientAuth, secret *vault.Secret)
}

var _ TokenRenewer = &VaultTokenRenewer{}

// getWatcherFunc is a function that returns a TokenWatcher and a function to stop the watcher
// this lets us hide away some go routines while testing
type getWatcherFunc func(client *vault.Client, secret *vault.Secret, watcherIncrement int) (TokenWatcher, error)

// VaultTokenRewner is a struct that implements the TokenRenewer interface in a manner based on the vault examples
// https://github.com/hashicorp/vault-examples/blob/main/examples/token-renewal/go/example.go
type VaultTokenRenewer struct {
	// leaseIncrement is the amount of time in seconds for which the lease should be renewed, passed to the NewLifetimeWatcher function
	// via LifetimeWatcherInput https://pkg.go.dev/github.com/hashicorp/vault/api#LifetimeWatcherInput
	leaseIncrement int
	// getWatcher is a function that returns a TokenWatcher and a function to stop the watcher. The default behavior is to call vaultGetWatcher
	// which returns a LifetimeWatcher and its Stop function
	getWatcher getWatcherFunc
	// retryOnNonRenewableSleep is the amount of time in seconds to sleep before retrying if the token is not renewable
	retryOnNonRenewableSleep int
}

type NewVaultTokenRenewerParams struct {
	// LeaseIncrement is the amount of time in seconds for which the lease should be renewed
	LeaseIncrement int
	// A function to provide the watcher and provide a point to inject a test function for testing.
	GetWatcher getWatcherFunc
	// retryOnNonRenewableSleep is the amount of time in seconds to sleep before retrying if the token is not renewable
	RetryOnNonRenewableSleep int
}

// NewVaultTokenRenewer returns a new VaultTokenRenewer and will set the default GetWatcher Function
func NewVaultTokenRenewer(params *NewVaultTokenRenewerParams) *VaultTokenRenewer {
	if params.GetWatcher == nil {
		params.GetWatcher = vaultGetWatcher
	}

	// This is the amount of time to sleep before retrying if the token is not renewable
	if params.RetryOnNonRenewableSleep == 0 {
		params.RetryOnNonRenewableSleep = 60
	}

	return &VaultTokenRenewer{
		leaseIncrement:           params.LeaseIncrement,
		getWatcher:               params.GetWatcher,
		retryOnNonRenewableSleep: params.RetryOnNonRenewableSleep,
	}
}

// ManageTokenRenewal wraps the renewal process in a go routine
func (t *VaultTokenRenewer) ManageTokenRenewal(ctx context.Context, client *vault.Client, clientAuth ClientAuth, secret *vault.Secret) {
	go t.RenewToken(ctx, client, clientAuth, secret)
}

// Once you've set the token for your Vault client, you will need to periodically renew its lease.
// taken from https://github.com/hashicorp/vault-examples/blob/main/examples/token-renewal/go/example.go
// the error that gets returned is dropped by the goroutine that calls this function, but is useful for testing
func (r *VaultTokenRenewer) RenewToken(ctx context.Context, client *vault.Client, clientAuth ClientAuth, secret *vault.Secret) error {
	contextutils.LoggerFrom(ctx).Debugf("Starting renewToken goroutine")

	for {
		// The first time we enter this loop, we will have a secret, and after that we will need to get a new one before we restart.
		// this function blocks until a renewal fails or the context is cancelled.
		retry, tokenErr := r.manageTokenLifecycle(ctx, client, secret)
		if tokenErr != nil {
			contextutils.LoggerFrom(ctx).Errorf("unable to start managing token lifecycle: %v.", tokenErr)
		}

		if !retry {
			return tokenErr
		}

		var err error
		// We're going to loop back around to the top of the for loop, so get a new secret
		secret, err = client.Auth().Login(ctx, clientAuth)
		if err != nil {
			contextutils.LoggerFrom(ctx).Errorf("unable to authenticate to Vault: %v.", err)
			return err // we are now no longer renewing the token
		}
	}

}

var vaultGetWatcher = getWatcherFunc(func(client *vault.Client, secret *vault.Secret, watcherIncrement int) (TokenWatcher, error) {
	watcher, err := client.NewLifetimeWatcher(&vault.LifetimeWatcherInput{
		Secret:    secret,
		Increment: watcherIncrement,
		// Below this comment we are manually setting the parameters to current defaults to protect against future changes
		Rand:          rand.New(rand.NewSource(int64(time.Now().Nanosecond()))),
		RenewBuffer:   5, // equivalent to vault.DefaultLifetimeWatcherRenewBuffer,
		RenewBehavior: vault.RenewBehaviorIgnoreErrors,
	})

	if err != nil {
		return nil, err
	}

	return watcher, nil
})

// Starts token lifecycle management
// otherwise returns nil so we can attempt login again.
// based on https://github.com/hashicorp/vault-examples/blob/main/examples/token-renewal/go/example.go
func (r *VaultTokenRenewer) manageTokenLifecycle(ctx context.Context, client *vault.Client, secret *vault.Secret) (bool, error) {

	// Make sure the token is renewable
	if renewable, err := secret.TokenIsRenewable(); !renewable || err != nil {
		// If the token is not renewable and we immediately try to renew it, we will just keep trying and hitting the same error
		// So we need to throw in a sleep

		contextutils.LoggerFrom(ctx).Errorw("Token is not configured to be renewable.", "retry", r.retryOnNonRenewableSleep, "Error", err, "TokenIsRenewable", renewable)

		// Create a timer and wait until its done or the context is cancelled
		timer := time.NewTimer(time.Duration(r.retryOnNonRenewableSleep) * time.Second)
		defer timer.Stop()

		for {
			select {
			case <-ctx.Done(): // context cancelled, don't retry
				return false, nil
			case <-timer.C: // timer expired, retry
				return true, nil
			}
		}
	}

	watcher, err := r.getWatcher(client, secret, r.leaseIncrement)
	// The only errors the constructor can return are if the input parameter is nil or if the secret is nil, and we
	// are always passing input and have validated the secret is not nil in the calling
	if err != nil {
		return false, ErrInitializeWatcher(err)
	}

	go watcher.Start()
	defer watcher.Stop()

	for {
		select {
		// `DoneCh` will return if renewal fails, or if the remaining lease
		// duration is under a built-in threshold and either renewing is not
		// extending it or renewing is disabled. In any case, the caller
		// needs to attempt to log in again.
		case err := <-watcher.DoneCh():
			utils.Measure(ctx, MLastRenewFailure, time.Now().Unix())
			utils.MeasureOne(ctx, MRenewFailures)
			if err != nil {
				contextutils.LoggerFrom(ctx).Debugf("Failed to renew token: %v. Re-attempting login.", err)
				return true, nil
			}
			// This occurs once the token has reached max TTL.
			contextutils.LoggerFrom(ctx).Debugf("Token can no longer be renewed. Re-attempting login.")
			return true, nil

		// Successfully completed renewal
		case renewal := <-watcher.RenewCh():
			utils.Measure(ctx, MLastRenewSuccess, time.Now().Unix())
			utils.MeasureOne(ctx, MRenewSuccesses)
			contextutils.LoggerFrom(ctx).Debugf("Successfully renewed: %v.", renewal)

		case <-ctx.Done():
			return false, nil
		}

	}
}
