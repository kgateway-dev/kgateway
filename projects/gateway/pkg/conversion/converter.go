package conversion

import (
	"context"
	"fmt"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewayv2 "github.com/solo-io/gloo/projects/gateway/pkg/api/v2"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	sk_errors "github.com/solo-io/solo-kit/pkg/errors"
	"go.uber.org/zap"
)

var (
	FailedToListGatewayResourcesError = func(err error, version, namespace string) error {
		return errors.Wrapf(err, "Failed to list %v gateway resources in %v", version, namespace)
	}

	FailedToReadExistingGatewayError = func(err error, version, namespace, name string) error {
		return errors.Wrapf(err, "Failed to read %v gateway %v.%v", version, namespace, name)
	}

	FailedToWriteGatewayError = func(err error, version, namespace, name string) error {
		return errors.Wrapf(err, "Failed to write %v gateway %v.%v", version, namespace, name)
	}
)

type ResourceConverter interface {
	ConvertAll(ctx context.Context) error
}

type resourceConverter struct {
	namespace        string
	v1GatewayClient  gatewayv1.GatewayClient
	v2GatewayClient  gatewayv2.GatewayClient
	gatewayConverter GatewayConverter
}

func NewResourceConverter(
	namespace string,
	v1GatewayClient gatewayv1.GatewayClient,
	v2GatewayClient gatewayv2.GatewayClient,
	gatewayConverter GatewayConverter,
) ResourceConverter {

	return &resourceConverter{
		namespace:        namespace,
		v1GatewayClient:  v1GatewayClient,
		v2GatewayClient:  v2GatewayClient,
		gatewayConverter: gatewayConverter,
	}
}

func (c *resourceConverter) ConvertAll(ctx context.Context) error {
	v1List, err := c.v1GatewayClient.List(c.namespace, clients.ListOpts{Ctx: ctx})
	if err != nil {
		wrapped := FailedToListGatewayResourcesError(err, "v1", c.namespace)
		contextutils.LoggerFrom(ctx).Errorw(wrapped.Error(), zap.Error(err), zap.String("namespace", c.namespace))
		return wrapped
	}

	var errs *multierror.Error
	for _, oldGateway := range v1List {
		convertedGateway := c.gatewayConverter.FromV1ToV2(oldGateway)

		overwriteExisting := false
		existing, err := c.v2GatewayClient.Read(c.namespace, convertedGateway.GetMetadata().Name, clients.ReadOpts{Ctx: ctx})
		if err != nil && !sk_errors.IsNotExist(err) {
			wrapped := FailedToReadExistingGatewayError(
				err,
				"v2",
				convertedGateway.GetMetadata().Namespace,
				convertedGateway.GetMetadata().Name)
			contextutils.LoggerFrom(ctx).Errorw(wrapped.Error(), zap.Error(err))
			errs = multierror.Append(errs, wrapped)
			continue
		} else if existing != nil {
			if existing.Metadata.Annotations[defaults.OriginKey] == defaults.ConvertedValue {
				// If the resource has already been converted, do not risk overwriting changes made since conversion.
				msg := fmt.Sprintf("Not writing converted gateway %v.%v: already written as converted.",
					existing.GetMetadata().Namespace,
					existing.GetMetadata().Name)
				contextutils.LoggerFrom(ctx).Info(msg)
				continue
			}
			if existing.Metadata.Annotations[defaults.OriginKey] == defaults.DefaultValue {
				// If the resource was written to v2 as a default, overwrite it.
				overwriteExisting = true
			}
		}

		if _, err := c.v2GatewayClient.Write(convertedGateway, clients.WriteOpts{Ctx: ctx, OverwriteExisting: overwriteExisting}); err != nil {
			wrapped := FailedToWriteGatewayError(
				err,
				"v2",
				convertedGateway.GetMetadata().Namespace,
				convertedGateway.GetMetadata().Name)
			contextutils.LoggerFrom(ctx).Errorw(wrapped.Error(), zap.Error(err), zap.Any("gateway", convertedGateway))
			errs = multierror.Append(errs, wrapped)
		} else {
			contextutils.LoggerFrom(ctx).Infow("Successfully wrote v2 gateway", zap.Any("gateway", convertedGateway))
		}
	}
	return errs.ErrorOrNil()
}
