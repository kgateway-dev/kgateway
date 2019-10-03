package validation

import (
	"context"
	"sync"

	"github.com/avast/retry-go"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/errors"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ClientConstructor func() (client validation.ProxyValidationServiceClient, e error)

// connectionRefreshingValidationClient wraps a validation.ProxyValidationServiceClient (grpc Connection)
// if a connection error occurs during an api call, the connectionRefreshingValidationClient
// attempts to reestablish the connection & retry the call before returning the error
type connectionRefreshingValidationClient struct {
	lock                      sync.RWMutex
	validationClient          validation.ProxyValidationServiceClient
	constructValidationClient ClientConstructor
}

// the constructor returned here is not threadsafe; call from a lock
func RetryOnUnavailableClientConstructor(ctx context.Context, serverAddress string) ClientConstructor {
	var cancel = func() {}
	return func() (client validation.ProxyValidationServiceClient, e error) {
		// cancel the previous client if it exists
		cancel()
		contextutils.LoggerFrom(ctx).Infow("starting proxy validation client... this may take a moment",
			zap.String("validation_server", serverAddress))
		var clientCtx context.Context
		clientCtx, cancel = context.WithCancel(ctx)

		cc, err := grpc.DialContext(clientCtx, serverAddress, grpc.WithInsecure(), grpc.WithBlock())
		if err != nil {
			return nil, errors.Wrapf(err, "failed to initialize grpc connection to validation server.")
		}

		return validation.NewProxyValidationServiceClient(cc), nil
	}
}

func NewConnectionRefreshingValidationClient(constructValidationClient func() (validation.ProxyValidationServiceClient, error)) (*connectionRefreshingValidationClient, error) {
	vc, err := constructValidationClient()
	if err != nil {
		return nil, err
	}
	return &connectionRefreshingValidationClient{
		constructValidationClient: constructValidationClient,
		validationClient:          vc,
	}, nil
}

func (c *connectionRefreshingValidationClient) ValidateProxy(ctx context.Context, proxy *validation.ProxyValidationServiceRequest, opts ...grpc.CallOption) (*validation.ProxyValidationServiceResponse, error) {
	ctx = contextutils.WithLogger(ctx, "robust-validation-client")

	var validationClient validation.ProxyValidationServiceClient
	var proxyReport *validation.ProxyValidationServiceResponse
	var reinstantiateClientErr error
	if err := retry.Do(func() error {
		c.lock.RLock()
		defer c.lock.RUnlock()
		validationClient = c.validationClient
		var err error
		proxyReport, err = validationClient.ValidateProxy(ctx, proxy, opts...)
		return err
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
	})); err != nil {
		return nil, err
	}
	return proxyReport, nil
}

func isUnavailableErr(err error) bool {
	switch status.Code(err) {
	case codes.Unavailable, codes.FailedPrecondition, codes.Aborted:
		return true
	}
	return false
}
