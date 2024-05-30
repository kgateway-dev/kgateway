package query

import (
	"context"
	"errors"
	"fmt"

	solokubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/utils"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type ListenerOptionQueries interface {
	// GetAttachedListenerOptions returns a slice of ListenerOption resources attached to a gateway on which
	// the listener resides and have either targeted the listener with section name or omitted section name.
	// The returned ListenerOption list is sorted by specificity in the order of
	//
	// - older with section name
	//
	// - newer with section name
	//
	// - older without section name
	//
	// - newer without section name
	//
	// Note that currently, only ListenerOptions in the same namespace as the Gateway can be attached.
	GetAttachedListenerOptions(ctx context.Context, listener *gwv1.Listener, parentGw *gwv1.Gateway) ([]*solokubev1.ListenerOption, error)
}

type listenerOptionQueries struct {
	c client.Client
}

type listenerOptionsQueryResult struct {
	optsWithSectionName    []*solokubev1.ListenerOption
	optsWithoutSectionName []*solokubev1.ListenerOption
}

func NewQuery(c client.Client) ListenerOptionQueries {
	return &listenerOptionQueries{c}
}

func (r *listenerOptionQueries) GetAttachedListenerOptions(
	ctx context.Context,
	listener *gwv1.Listener,
	parentGw *gwv1.Gateway) ([]*solokubev1.ListenerOption, error) {
	if parentGw == nil {
		return nil, errors.New("nil parent gateway")
	}
	if parentGw.GetName() == "" || parentGw.GetNamespace() == "" {
		return nil, fmt.Errorf("parent gateway must have name and namespace; received name: %s, namespace: %s", parentGw.GetName(), parentGw.GetNamespace())
	}
	nn := types.NamespacedName{
		Namespace: parentGw.Namespace,
		Name:      parentGw.Name,
	}
	list := &solokubev1.ListenerOptionList{}
	if err := r.c.List(
		ctx,
		list,
		client.MatchingFieldsSelector{Selector: fields.OneTermEqualSelector(ListenerOptionTargetField, nn.String())},
		client.InNamespace(parentGw.GetNamespace()),
	); err != nil {
		return nil, err
	}

	if len(list.Items) == 0 {
		return nil, nil
	}

	attachedItems := &listenerOptionsQueryResult{}

	for i := range list.Items {
		targetRefs := list.Items[i].Spec.GetTargetRef()
		if len(targetRefs) > 1 {
			//TODO: warning that multiple refs present, only using first one
		}
		if sectionName := targetRefs[0].GetSectionName(); sectionName != nil && sectionName.GetValue() != "" {
			// We have a section name, now check if it matches the specific listener provided
			if sectionName.GetValue() == string(listener.Name) {
				attachedItems.optsWithSectionName = append(attachedItems.optsWithSectionName, &list.Items[i])
			}
		} else {
			// Attach all matched items that do not have a section name and let the caller be discerning
			attachedItems.optsWithoutSectionName = append(attachedItems.optsWithoutSectionName, &list.Items[i])
		}
	}

	// This can happen if the only ListenerOption resources returned by List target other Listeners by section name
	if len(attachedItems.optsWithoutSectionName)+len(attachedItems.optsWithSectionName) == 0 {
		return nil, nil
	}

	utils.SortByCreationTime(attachedItems.optsWithSectionName)
	utils.SortByCreationTime(attachedItems.optsWithoutSectionName)
	return append(attachedItems.optsWithSectionName, attachedItems.optsWithoutSectionName...), nil
}
