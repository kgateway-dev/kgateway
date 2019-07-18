package conversion

import (
	"context"

	"github.com/pkg/errors"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/api/v2alpha1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

var (
	FailedToListGatewayResourcesError = func(err error, version, namespace string) error {
		return errors.Wrapf(err, "Failed to list %v gateway resources in %v", version, namespace)
	}

	FailedToDeleteGatewayError = func(err error, version, namespace, name string) error {
		return errors.Wrapf(err, "Failed to delete %v gateway %v.%v", version, namespace, name)
	}

	FailedToWriteGatewayError = func(err error, version, namespace, name string) error {
		return errors.Wrapf(err, "Failed to write %v gateway %v.%v", version, namespace, name)
	}
)

type Ladder interface {
	Climb() error
}

type ladder struct {
	ctx              context.Context
	namespace        string
	v1Client         v1.GatewayClient
	v2alpha1Client   v2alpha1.GatewayClient
	gatewayConverter GatewayConverter
}

func NewLadder(
	ctx context.Context,
	namespace string,
	v1Client v1.GatewayClient,
	v2alpha1Client v2alpha1.GatewayClient,
	gatewayConverter GatewayConverter,
) Ladder {

	return &ladder{
		ctx:              ctx,
		namespace:        namespace,
		v1Client:         v1Client,
		v2alpha1Client:   v2alpha1Client,
		gatewayConverter: gatewayConverter,
	}
}

func (c *ladder) Climb() error {
	v1List, err := c.v1Client.List(c.namespace, clients.ListOpts{Ctx: c.ctx})
	if err != nil {
		wrapped := FailedToListGatewayResourcesError(err, "v1", c.namespace)
		contextutils.LoggerFrom(c.ctx).Errorw(wrapped.Error(), zap.Error(err), zap.String("namespace", c.namespace))
	}

	var g errgroup.Group
	for _, oldGateway := range v1List {
		g.Go(func() error {
			convertedGateway := c.gatewayConverter.FromV1ToV2alpha1(oldGateway)

			if err := c.v1Client.Delete(
				oldGateway.GetMetadata().Namespace,
				oldGateway.GetMetadata().Name,
				clients.DeleteOpts{Ctx: c.ctx}); err != nil {

				wrapped := FailedToDeleteGatewayError(
					err,
					"v1",
					oldGateway.GetMetadata().Namespace,
					oldGateway.GetMetadata().Name)
				contextutils.LoggerFrom(c.ctx).Errorw(wrapped.Error(), zap.Error(err), zap.Any("gateway", oldGateway))
				return wrapped
			}
			contextutils.LoggerFrom(c.ctx).Infow("Successfully deleted v1 Gateway", zap.Any("gateway", oldGateway))

			if _, err := c.v2alpha1Client.Write(convertedGateway, clients.WriteOpts{Ctx: c.ctx}); err != nil {
				wrapped := FailedToWriteGatewayError(
					err,
					"v2alpha1",
					convertedGateway.GetMetadata().Namespace,
					convertedGateway.GetMetadata().Name)
				contextutils.LoggerFrom(c.ctx).Errorw(wrapped.Error(), zap.Error(err), zap.Any("gateway", convertedGateway))
				return wrapped
			}
			contextutils.LoggerFrom(c.ctx).Infow("Successfully wrote v2alpha1 gateway", zap.Any("gateway", convertedGateway))
			return nil
		})
	}
	return g.Wait()
}
