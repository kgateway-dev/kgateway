package selectionutils

import (
	"context"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	sk_errors "github.com/solo-io/solo-kit/pkg/errors"
)

type RouteTableSelector interface {
	SelectOrBuildRouteTable(ctx context.Context, ref *core.ResourceRef) (*gatewayv1.RouteTable, error)
}

type routeTableSelector struct {
	client       gatewayv1.RouteTableClient
	podNamespace string
}

var _ RouteTableSelector = &routeTableSelector{}

func NewRouteTableSelector(client gatewayv1.RouteTableClient, podNamespace string) *routeTableSelector {
	return &routeTableSelector{
		client:       client,
		podNamespace: podNamespace,
	}
}

func (s *routeTableSelector) SelectOrBuildRouteTable(ctx context.Context, ref *core.ResourceRef) (*gatewayv1.RouteTable, error) {
	// Read or build route table
	// unlike virtual service, name must be provided as there is no "default" virtual service
	name := ref.GetName()
	if name == "" {
		return nil, eris.New("must provide a name for the target route table")
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

	// Build a new default route table object
	return s.build(ctx, ref)
}

func (s *routeTableSelector) build(ctx context.Context, ref *core.ResourceRef) (*gatewayv1.RouteTable, error) {
	routeTable := &gatewayv1.RouteTable{
		Metadata: &core.Metadata{
			Namespace: ref.GetNamespace(),
			Name:      ref.GetName(),
		},
	}
	if routeTable.GetMetadata().GetNamespace() == "" {
		routeTable.GetMetadata().Namespace = s.podNamespace
	}
	if routeTable.GetMetadata().GetName() == "" {
		routeTable.GetMetadata().Name = "default"
	}

	return routeTable, nil
}
