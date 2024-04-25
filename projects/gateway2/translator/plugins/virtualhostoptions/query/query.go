package query

import (
	"context"

	"github.com/rotisserie/eris"
	solokubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/utils"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type VirtualHostOptionQueries interface {
	// GetVirtualHostOptionsForListener returns a slice of VirtualHostOption resources attached to a gateway on which
	// the listener resides and have either targeted the listener with section name or omitted section name.
	// The returned VirtualHostOption list is sorted by specificity in the order of
	//
	// - older with section name
	//
	// - newer with section name
	//
	// - older without section name
	//
	// - newer without section name
	//
	// Note that currently, only VirtualHostOptions in the same namespace as the Gateway can be attached.
	GetVirtualHostOptionsForListener(ctx context.Context, listener *gwv1.Listener, parentGw *gwv1.Gateway) ([]*solokubev1.VirtualHostOption, error)
}

type virtualHostOptionQueries struct {
	c client.Client
}

type virtualHostOptionsQueryResult struct {
	optsWithSectionName    []*solokubev1.VirtualHostOption
	optsWithoutSectionName []*solokubev1.VirtualHostOption
}

func NewQuery(c client.Client) VirtualHostOptionQueries {
	return &virtualHostOptionQueries{c}
}

func (r *virtualHostOptionQueries) GetVirtualHostOptionsForListener(
	ctx context.Context,
	listener *gwv1.Listener,
	parentGw *gwv1.Gateway) ([]*solokubev1.VirtualHostOption, error) {
	if parentGw == nil {
		return nil, eris.New("nil parent gateway")
	}
	if parentGw.GetName() == "" || parentGw.GetNamespace() == "" {
		return nil, eris.Errorf("parent gateway must have name and namespace; received name: %s, namespace: %s", parentGw.GetName(), parentGw.GetNamespace())
	}
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

	attachedItems := &virtualHostOptionsQueryResult{}

	for i := range list.Items {
		if sectionName := list.Items[i].Spec.GetTargetRef().GetSectionName(); sectionName != nil && sectionName.GetValue() != "" {
			// We have a section name, now check if it matches our expectation
			if sectionName.GetValue() == string(listener.Name) {
				attachedItems.optsWithSectionName = append(attachedItems.optsWithSectionName, &list.Items[i])
			}
		} else {
			// Attach all matched items that do not have a section name and let the caller be discerning
			attachedItems.optsWithoutSectionName = append(attachedItems.optsWithoutSectionName, &list.Items[i])
		}
	}

	// This can happen if the only VirtualHostOption resources returned by List target other Listeners by section name
	if len(attachedItems.optsWithoutSectionName)+len(attachedItems.optsWithSectionName) == 0 {
		return nil, nil
	}

	utils.SortByCreationTime(attachedItems.optsWithSectionName)
	utils.SortByCreationTime(attachedItems.optsWithoutSectionName)
	return append(attachedItems.optsWithSectionName, attachedItems.optsWithoutSectionName...), nil
}
