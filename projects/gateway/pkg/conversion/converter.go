package conversion

import (
	"context"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/api/v2alpha1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"go.uber.org/zap"
)

var (
	FailedToListGatewayResourcesError = func(err error, version, namespace string) error {
		return errors.Wrapf(err, "Failed to list %v gateway resources in %v", version, namespace)
	}

	FailedToWriteGatewayError = func(err error, version, namespace, name string) error {
		return errors.Wrapf(err, "Failed to write %v gateway %v.%v", version, namespace, name)
	}
)

type ResourceConverter interface {
	ConvertAll() error
}

type resourceConverter struct {
	ctx                   context.Context
	namespace             string
	v1GatewayClient       v1.GatewayClient
	v2alpha1GatewayClient v2alpha1.GatewayClient
	gatewayConverter      GatewayConverter
}

func NewResourceConverter(
	ctx context.Context,
	namespace string,
	v1GatewayClient v1.GatewayClient,
	v2alpha1GatewayClient v2alpha1.GatewayClient,
	gatewayConverter GatewayConverter,
) ResourceConverter {

	return &resourceConverter{
		ctx:                   ctx,
		namespace:             namespace,
		v1GatewayClient:       v1GatewayClient,
		v2alpha1GatewayClient: v2alpha1GatewayClient,
		gatewayConverter:      gatewayConverter,
	}
}

func (c *resourceConverter) ConvertAll() error {
	v1List, err := c.v1GatewayClient.List(c.namespace, clients.ListOpts{Ctx: c.ctx})
	if err != nil {
		wrapped := FailedToListGatewayResourcesError(err, "v1", c.namespace)
		contextutils.LoggerFrom(c.ctx).Errorw(wrapped.Error(), zap.Error(err), zap.String("namespace", c.namespace))
		return wrapped
	}

	var writeErrors *multierror.Error
	for _, oldGateway := range v1List {
		convertedGateway := c.gatewayConverter.FromV1ToV2alpha1(oldGateway)
		if _, err := c.v2alpha1GatewayClient.Write(convertedGateway, clients.WriteOpts{Ctx: c.ctx}); err != nil {
			wrapped := FailedToWriteGatewayError(
				err,
				"v2alpha1",
				convertedGateway.GetMetadata().Namespace,
				convertedGateway.GetMetadata().Name)
			contextutils.LoggerFrom(c.ctx).Errorw(wrapped.Error(), zap.Error(err), zap.Any("gateway", convertedGateway))
			writeErrors = multierror.Append(writeErrors, wrapped)
		} else {
			contextutils.LoggerFrom(c.ctx).Infow("Successfully wrote v2alpha1 gateway", zap.Any("gateway", convertedGateway))
		}
	}
	return writeErrors.ErrorOrNil()
}
