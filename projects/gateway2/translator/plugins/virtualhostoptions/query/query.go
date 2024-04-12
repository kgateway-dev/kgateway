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
	// GetVirtualHostOptionsForListener returns a VirtualHostOptionsList with the VirtualHostOption resources attached
	// to the provided Listener's parent Gateway, preferring an option that explicitly targets the listener in sectionName
	// if applicable. Note that currently, only VirtualHostOptions in the same namespace as the Gateway can be attached.
	GetVirtualHostOptionsForListener(ctx context.Context, listener *gwv1.Listener, parentGw *gwv1.Gateway) (*VirtualHostOptionsQueryResult, error)
}

type virtualHostOptionQueries struct {
	c client.Client
}

type VirtualHostOptionsQueryResult struct {
	OptsWithSectionName    []*solokubev1.VirtualHostOption
	OptsWithoutSectionName []*solokubev1.VirtualHostOption
}

func NewQuery(c client.Client) VirtualHostOptionQueries {
	return &virtualHostOptionQueries{c}
}

func (r *virtualHostOptionQueries) GetVirtualHostOptionsForListener(
	ctx context.Context,
	listener *gwv1.Listener,
	parentGw *gwv1.Gateway) (*VirtualHostOptionsQueryResult, error) {
	nn := types.NamespacedName{
		Namespace: parentGw.Namespace,
		Name:      parentGw.Name,
	}
	list := &solokubev1.VirtualHostOptionList{}
	if err := r.c.List(
		ctx,
		list,
		client.MatchingFieldsSelector{Selector: fields.OneTermEqualSelector(VirtualHostOptionTargetField, nn.String())},
		client.InNamespace(parentGw.GetNamespace()),
	); err != nil {
		return nil, err
	}

	if len(list.Items) == 0 {
		return nil, nil
	}

	attachedItems := &VirtualHostOptionsQueryResult{}

	for i := range list.Items {
		if sectionName := list.Items[i].Spec.GetTargetRef().GetSectionName(); sectionName != nil && sectionName.GetValue() != "" {
			// We have a section name, now check if it matches our expectation
			if sectionName.GetValue() == string(listener.Name) {
				attachedItems.OptsWithSectionName = append(attachedItems.OptsWithSectionName, &list.Items[i])
			}
		} else {
			// Attach all matched items that do not have a section name and let the caller be discerning
			attachedItems.OptsWithoutSectionName = append(attachedItems.OptsWithoutSectionName, &list.Items[i])
		}
	}

	if len(attachedItems.OptsWithoutSectionName)+len(attachedItems.OptsWithSectionName) == 0 {
		return nil, nil
	}
	return attachedItems, nil
}
