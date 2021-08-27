package validation

import (
	"context"
	"sync"

	"github.com/avast/retry-go"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ClientConstructor func() (client validation.GlooValidationServiceClient, e error)

// connectionRefreshingValidationClient wraps a validation.GlooValidationServiceClient (grpc Connection)
// if a connection error occurs during an api call, the connectionRefreshingValidationClient
// attempts to reestablish the connection & retry the call before returning the error
type connectionRefreshingValidationClient struct {
	lock                      sync.RWMutex
	validationClient          validation.GlooValidationServiceClient
	constructValidationClient ClientConstructor
}

// the constructor returned here is not threadsafe; call from a lock
func RetryOnUnavailableClientConstructor(ctx context.Context, serverAddress string) ClientConstructor {
	var cancel = func() {}
	return func() (client validation.GlooValidationServiceClient, e error) {
		// cancel the previous client if it exists
		cancel()
		contextutils.LoggerFrom(ctx).Infow("starting gloo validation client... this may take a moment",
			zap.String("validation_server", serverAddress))
		var clientCtx context.Context
		clientCtx, cancel = context.WithCancel(ctx)

		cc, err := grpc.DialContext(clientCtx, serverAddress, grpc.WithInsecure(), grpc.WithBlock())
		if err != nil {
			return nil, eris.Wrapf(err, "failed to initialize grpc connection to validation server.")
		}

		return validation.NewGlooValidationServiceClient(cc), nil
	}
}

func NewConnectionRefreshingValidationClient(constructValidationClient func() (validation.GlooValidationServiceClient, error)) (*connectionRefreshingValidationClient, error) {
	vc, err := constructValidationClient()
	if err != nil {
		return nil, err
	}
	return &connectionRefreshingValidationClient{
		constructValidationClient: constructValidationClient,
		validationClient:          vc,
	}, nil
}

func (c *connectionRefreshingValidationClient) Validate(ctx context.Context, proxy *validation.GlooValidationServiceRequest, opts ...grpc.CallOption) (*validation.GlooValidationServiceResponse, error) {
	ctx = contextutils.WithLogger(ctx, "retrying-validation-client")

	var proxyReport *validation.GlooValidationServiceResponse

	return proxyReport, c.retryWithNewClient(ctx, func(validationClient validation.GlooValidationServiceClient) error {
		var err error
		proxyReport, err = validationClient.Validate(ctx, proxy, opts...)
		return err
	})
}

func (c *connectionRefreshingValidationClient) NotifyOnResync(ctx context.Context, in *validation.NotifyOnResyncRequest, opts ...grpc.CallOption) (validation.GlooValidationService_NotifyOnResyncClient, error) {
	var notifier validation.GlooValidationService_NotifyOnResyncClient

	return notifier, c.retryWithNewClient(ctx, func(validationClient validation.GlooValidationServiceClient) error {
		var err error
		notifier, err = validationClient.NotifyOnResync(ctx, in, opts...)
		return err
	})
}

func (c *connectionRefreshingValidationClient) retryWithNewClient(ctx context.Context, fn func(validationClient validation.GlooValidationServiceClient) error) error {
	var validationClient validation.GlooValidationServiceClient
	var reinstantiateClientErr error
	return retry.Do(func() error {
		c.lock.RLock()
		defer c.lock.RUnlock()
		validationClient = c.validationClient
		return fn(validationClient)
	}, retry.RetryIf(func(e error) bool {
		if reinstantiateClientErr != nil {
			contextutils.LoggerFrom(ctx).Warnw("failed to create new validation client during retry", zap.Error(reinstantiateClientErr))
			return false
		}
		return isUnavailableErr(e)
	}), retry.OnRetry(func(n uint, err error) {
		if !isUnavailableErr(err) {
			return
		}
		c.lock.Lock()
		defer c.lock.Unlock()
		// if someone already changed my client, do not replace it
		if validationClient == c.validationClient {
			c.validationClient, reinstantiateClientErr = c.constructValidationClient()
		}
	}))
}

func isUnavailableErr(err error) bool {
	switch status.Code(err) {
	case codes.Unavailable, codes.FailedPrecondition, codes.Aborted:
		return true
	}
	return false
}
