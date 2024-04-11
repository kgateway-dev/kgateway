package query

import (
	"context"

	solokubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type VirtualHostOptionQueries interface {
	// Populates the provided VirtualHostOptionList with the VirtualHostOption resources attached to the provided Gateway.
	// Note that currently, only VirtualHostOptions in the same namespace as the Gateway can be attached.
	GetVirtualHostOptionsForGateway(ctx context.Context, gw *gwv1.Gateway) (*solokubev1.VirtualHostOptionList, error)
}

type virtualHostOptionQueries struct {
	c client.Client
}

func NewQuery(c client.Client) VirtualHostOptionQueries {
	return &virtualHostOptionQueries{c}
}

func (r *virtualHostOptionQueries) GetVirtualHostOptionsForGateway(ctx context.Context, gw *gwv1.Gateway) (*solokubev1.VirtualHostOptionList, error) {
	nn := types.NamespacedName{
		Namespace: gw.Namespace,
		Name:      gw.Name,
	}
	list := &solokubev1.VirtualHostOptionList{}
	if err := r.c.List(
		ctx,
		list,
		client.MatchingFieldsSelector{Selector: fields.OneTermEqualSelector(VirtualHostOptionTargetField, nn.String())},
		client.InNamespace(gw.GetNamespace()),
	); err != nil {
		return nil, err
	}

	return list, nil
}
