package selectionutils

import (
	"context"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/go-utils/errors"

	"github.com/solo-io/gloo/pkg/listers"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	sk_errors "github.com/solo-io/solo-kit/pkg/errors"
	"go.uber.org/zap"
)

type RouteTableSelector interface {
	SelectOrCreateRouteTable(ctx context.Context, ref *core.ResourceRef) (*gatewayv1.RouteTable, error)
}

type routeTableSelector struct {
	client          gatewayv1.RouteTableClient
	namespaceLister listers.NamespaceLister
	podNamespace    string
}

var _ RouteTableSelector = &routeTableSelector{}

func NewRouteTableSelector(client gatewayv1.RouteTableClient, namespaceLister listers.NamespaceLister, podNamespace string) *routeTableSelector {
	return &routeTableSelector{
		client:          client,
		namespaceLister: namespaceLister,
		podNamespace:    podNamespace,
	}
}

func (s *routeTableSelector) SelectOrCreateRouteTable(ctx context.Context, ref *core.ResourceRef) (*gatewayv1.RouteTable, error) {
	// Read or create route table
	// unlike virtual service, name must be provided as there is no "default" virtual service
	name := ref.GetName()
	if name == "" {
		return nil, errors.New("must provide a name for the target route table")
	}

	ns := ref.GetNamespace()
	if ns == "" {
		ns = defaults.GlooSystem
	}
	found, err := s.client.Read(ref.GetNamespace(), ref.GetName(), clients.ReadOpts{Ctx: ctx})
	if err != nil && !sk_errors.IsNotExist(err) {
		return nil, err
	}
	if found != nil {
		return found, nil
	}

	return s.create(ctx, ref)
}

func (s *routeTableSelector) create(ctx context.Context, ref *core.ResourceRef) (*gatewayv1.RouteTable, error) {
	routeTable := &gatewayv1.RouteTable{
		Metadata: core.Metadata{
			Namespace: ref.GetNamespace(),
			Name:      ref.GetName(),
		},
	}
	if routeTable.GetMetadata().Namespace == "" {
		routeTable.Metadata.Namespace = s.podNamespace
	}
	if routeTable.GetMetadata().Name == "" {
		routeTable.Metadata.Name = "default"
	}

	written, err := s.client.Write(routeTable, clients.WriteOpts{Ctx: ctx})
	if err != nil {
		return nil, err
	}
	contextutils.LoggerFrom(ctx).Infow("Created new default route table", zap.Any("routeTable", routeTable))
	return written, nil
}
