package selectionutils

import (
	"context"

	"github.com/solo-io/gloo/pkg/listers"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	sk_errors "github.com/solo-io/solo-kit/pkg/errors"
	"go.uber.org/zap"
)

//go:generate mockgen -destination mocks/mock_virtual_service.go -package mocks github.com/solo-io/gloo/pkg/utils/selectionutils VirtualServiceSelector

type VirtualServiceSelector interface {
	SelectOrBuildVirtualService(ctx context.Context, ref *core.ResourceRef) (*gatewayv1.VirtualService, error)
}

type virtualServiceSelector struct {
	client          gatewayv1.VirtualServiceClient
	namespaceLister listers.NamespaceLister
	podNamespace    string
}

var _ VirtualServiceSelector = &virtualServiceSelector{}

func NewVirtualServiceSelector(client gatewayv1.VirtualServiceClient, namespaceLister listers.NamespaceLister, podNamespace string) *virtualServiceSelector {
	return &virtualServiceSelector{
		client:          client,
		namespaceLister: namespaceLister,
		podNamespace:    podNamespace,
	}
}

func (s *virtualServiceSelector) SelectOrBuildVirtualService(ctx context.Context, ref *core.ResourceRef) (*gatewayv1.VirtualService, error) {
	// Read or build virtual service as specified
	if ref.GetNamespace() != "" && ref.GetName() != "" {
		found, err := s.client.Read(ref.GetNamespace(), ref.GetName(), clients.ReadOpts{Ctx: ctx})
		if err != nil && !sk_errors.IsNotExist(err) {
			return nil, err
		}
		if found != nil {
			return found, nil
		}
	}

	// Create a new default virtual service with the given name
	if ref.GetName() != "" {
		return s.build(ctx, ref)
	}

	// Look for an existing virtual service with * domain
	namespaces, err := s.namespaceLister.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, ns := range namespaces {
		allVirtualServices, err := s.client.List(ns, clients.ListOpts{Ctx: ctx})
		if err != nil {
			return nil, err
		}
		for _, vs := range allVirtualServices {
			for _, domain := range vs.GetVirtualHost().GetDomains() {
				if domain == "*" {
					contextutils.LoggerFrom(ctx).Infow("Selected virtual service with domain *", zap.Any("virtualService", vs))
					return vs, nil
				}
			}
		}
	}

	// Build a new default virtual service object
	return s.build(ctx, ref)
}

func (s *virtualServiceSelector) build(ctx context.Context, ref *core.ResourceRef) (*gatewayv1.VirtualService, error) {
	virtualService := &gatewayv1.VirtualService{
		Metadata: &core.Metadata{
			Namespace: ref.GetNamespace(),
			Name:      ref.GetName(),
		},
		VirtualHost: &gatewayv1.VirtualHost{
			Domains: []string{"*"},
		},
	}
	if virtualService.GetMetadata().GetNamespace() == "" {
		virtualService.GetMetadata().Namespace = s.podNamespace
	}
	if virtualService.GetMetadata().GetName() == "" {
		virtualService.GetMetadata().Name = "default"
	}

	return virtualService, nil
}
