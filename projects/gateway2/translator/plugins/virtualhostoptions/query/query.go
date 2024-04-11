package query

import (
	"context"

	"github.com/rotisserie/eris"
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
	GetVirtualHostOptionsForListener(ctx context.Context, listener *gwv1.Listener, parentGw *gwv1.Gateway) (*solokubev1.VirtualHostOption, error)
}

type virtualHostOptionQueries struct {
	c client.Client
}

func NewQuery(c client.Client) VirtualHostOptionQueries {
	return &virtualHostOptionQueries{c}
}

func (r *virtualHostOptionQueries) GetVirtualHostOptionsForListener(
	ctx context.Context,
	listener *gwv1.Listener,
	parentGw *gwv1.Gateway) (*solokubev1.VirtualHostOption, error) {
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
	attachedItems := make([]*solokubev1.VirtualHostOption, len(list.Items))
	for i := range list.Items {
		attachedItems[i] = &list.Items[i]
	}

	optsWithSectionName := map[string]*solokubev1.VirtualHostOption{}
	optsWithoutSectionName := []*solokubev1.VirtualHostOption{}
	for _, opt := range attachedItems {
		if sectionName := opt.Spec.GetTargetRef().GetSectionName(); sectionName != nil && sectionName.GetValue() != "" {
			optsWithSectionName[sectionName.GetValue()] = opt
		} else {
			optsWithoutSectionName = append(optsWithoutSectionName, opt)
		}
	}

	if len(optsWithoutSectionName) > 1 {
		return nil, eris.Errorf("expected 1 VirtualHostOption resource targeting Gateway (%s.%s); got %d", parentGw.Namespace, parentGw.Name, len(optsWithoutSectionName))
	}

	var optToUse *solokubev1.VirtualHostOption
	// If there is not a section name or the specified section name matches our listener, apply the vhost options
	if targetedOpt, ok := optsWithSectionName[string(listener.Name)]; ok {
		optToUse = targetedOpt
	} else if len(optsWithoutSectionName) == 1 {
		optToUse = optsWithoutSectionName[0]
	}

	return optToUse, nil
}
