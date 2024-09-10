package query

import (
	"context"
	"fmt"

	solokubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/utils"
	"github.com/solo-io/go-utils/contextutils"
	skv2corev1 "github.com/solo-io/skv2/pkg/api/core.skv2.solo.io/v1"
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

type listenerOptionPolicy struct {
	obj *solokubev1.ListenerOption
}

func (o listenerOptionPolicy) GetTargetRefs() []*skv2corev1.PolicyTargetReferenceWithSectionName {
	return o.obj.Spec.GetTargetRefs()
}

func (o listenerOptionPolicy) GetObject() *solokubev1.ListenerOption {
	return o.obj
}

func NewQuery(c client.Client) ListenerOptionQueries {
	return &listenerOptionQueries{c}
}

func (r *listenerOptionQueries) GetAttachedListenerOptions(
	ctx context.Context,
	listener *gwv1.Listener,
	parentGw *gwv1.Gateway,
) ([]*solokubev1.ListenerOption, error) {
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

	policies := buildWrapperType(ctx, list)
	orderedPolicies := utils.GetPrioritizedListenerPolicies(policies, listener)
	return orderedPolicies, nil
}

func buildWrapperType(
	ctx context.Context,
	list *solokubev1.ListenerOptionList,
) []utils.PolicyWithSectionedTargetRefs[*solokubev1.ListenerOption] {
	policies := []utils.PolicyWithSectionedTargetRefs[*solokubev1.ListenerOption]{}
	for i := range list.Items {
		item := &list.Items[i]

		// warn for multiple targetRefs until we actually support this
		// TODO: remove this as part of https://github.com/solo-io/solo-projects/issues/6286
		if len(item.Spec.GetTargetRefs()) > 1 {
			contextutils.LoggerFrom(ctx).Warnf(utils.MultipleTargetRefErrStr, item.GetNamespace(), item.GetName())
		}

		policy := listenerOptionPolicy{
			obj: item,
		}
		policies = append(policies, policy)
	}
	return policies
}
