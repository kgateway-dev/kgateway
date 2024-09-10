package query

import (
	"context"

	"github.com/hashicorp/go-multierror"
	"github.com/rotisserie/eris"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	solokubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	gwquery "github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/utils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	glooutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var routeOptionGK = schema.GroupKind{
	Group: sologatewayv1.RouteOptionGVK.Group,
	Kind:  sologatewayv1.RouteOptionGVK.Kind,
}

type RouteOptionQueries interface {
	// GetRouteOptionForRouteRule returns the RouteOption attached to the given route and rule.
	//
	// It performs a merge of the ExtensionRef filter attachment and targetRef based attachment
	// while giving priority to the ExtensionRef filter attachment.
	// A lower priority RouteOption may augment the top-level fields in a higher priority RouteOption,
	// but can never override them.
	//
	// When multiple RouteOptions are attached to the route via targetRef, only the earliest created
	// resource is considered for the merge.
	//
	// It returns the merged RouteOption, a list of sources corresponding to the merge, and an error if one occurs.
	GetRouteOptionForRouteRule(
		ctx context.Context,
		route types.NamespacedName,
		rule *gwv1.HTTPRouteRule,
		gwQueries gwquery.GatewayQueries,
	) (*solokubev1.RouteOption, []*gloov1.SourceMetadata_SourceRef, error)
}

type routeOptionQueries struct {
	c client.Client
}

func NewQuery(c client.Client) RouteOptionQueries {
	return &routeOptionQueries{c}
}

func (r *routeOptionQueries) GetRouteOptionForRouteRule(
	ctx context.Context,
	route types.NamespacedName,
	rule *gwv1.HTTPRouteRule,
	gwQueries gwquery.GatewayQueries,
) (*solokubev1.RouteOption, []*gloov1.SourceMetadata_SourceRef, error) {
	var sources []*gloov1.SourceMetadata_SourceRef
	merged := &solokubev1.RouteOption{}

	filterAttachments, err := lookupFilterAttachments(ctx, route, rule, gwQueries)
	if err != nil {
		return nil, nil, err
	}
	for _, opt := range filterAttachments {
		optionUsed := false
		merged.Spec.Options, optionUsed = glooutils.ShallowMergeRouteOptions(merged.Spec.GetOptions(), opt.Spec.GetOptions())
		if optionUsed {
			sources = append(sources, routeOptionToSourceRef(opt))
		}
	}

	var list solokubev1.RouteOptionList
	if err := r.c.List(
		ctx,
		&list,
		client.MatchingFieldsSelector{Selector: fields.OneTermEqualSelector(RouteOptionTargetField, route.String())},
		client.InNamespace(route.Namespace),
	); err != nil {
		return nil, nil, err
	}

	if len(list.Items) == 0 {
		return nilOptionIfEmpty(merged), sources, nil
	}

	// warn for multiple targetRefs until we actually support this
	// TODO: remove this as part of https://github.com/solo-io/solo-projects/issues/6286
	for i := range list.Items {
		item := &list.Items[i]
		if len(item.Spec.GetTargetRefs()) > 1 {
			contextutils.LoggerFrom(ctx).Warnf(utils.MultipleTargetRefErrStr, item.GetNamespace(), item.GetName())
		}
	}

	out := make([]*solokubev1.RouteOption, len(list.Items))
	for i := range list.Items {
		out[i] = &list.Items[i]
	}
	utils.SortByCreationTime(out)
	for _, opt := range out {
		optionUsed := false
		merged.Spec.Options, optionUsed = glooutils.ShallowMergeRouteOptions(merged.Spec.GetOptions(), opt.Spec.GetOptions())
		if optionUsed {
			sources = append(sources, routeOptionToSourceRef(opt))
		}
	}

	return nilOptionIfEmpty(merged), sources, nil
}

func nilOptionIfEmpty(opt *solokubev1.RouteOption) *solokubev1.RouteOption {
	if opt == nil || opt.Spec.GetOptions() == nil {
		return nil
	}
	return opt
}

// lookupFilterAttachments returns the RouteOptions attached to the route via ExtensionRef filters on the route's rule
func lookupFilterAttachments(
	ctx context.Context,
	route types.NamespacedName,
	rule *gwv1.HTTPRouteRule,
	gwQueries gwquery.GatewayQueries,
) ([]*solokubev1.RouteOption, error) {
	if rule == nil {
		return nil, nil
	}

	filters := utils.FindExtensionRefFilters(rule, routeOptionGK)
	if filters == nil {
		return nil, nil
	}

	var out []*solokubev1.RouteOption
	var multiErr *multierror.Error
	extLookup := extensionRefLookup{namespace: route.Namespace}
	for _, filter := range filters {
		routeOption, err := utils.GetExtensionRefObjFrom[*solokubev1.RouteOption](ctx, extLookup, gwQueries, filter.ExtensionRef)
		if err != nil {
			// If the filter is not found, report a specific error so that it can reflect more
			// clearly on the status of the HTTPRoute.
			if apierrors.IsNotFound(err) {
				multiErr = multierror.Append(multiErr, errFilterNotFound(route.Namespace, &filter))
			} else {
				multiErr = multierror.Append(multiErr, err)
			}
			continue
		}
		out = append(out, routeOption)
	}

	return out, multiErr.ErrorOrNil()
}

type extensionRefLookup struct {
	namespace string
}

func (e extensionRefLookup) GroupKind() (metav1.GroupKind, error) {
	return metav1.GroupKind{
		Group: routeOptionGK.Group,
		Kind:  routeOptionGK.Kind,
	}, nil
}

func (e extensionRefLookup) Namespace() string {
	return e.namespace
}

func errFilterNotFound(namespace string, filter *gwv1.HTTPRouteFilter) error {
	return eris.Errorf(
		"extensionRef '%s' of type %s.%s in namespace '%s' not found",
		filter.ExtensionRef.Group,
		filter.ExtensionRef.Kind,
		filter.ExtensionRef.Name,
		namespace,
	)
}

func routeOptionToSourceRef(opt *solokubev1.RouteOption) *gloov1.SourceMetadata_SourceRef {
	return &gloov1.SourceMetadata_SourceRef{
		ResourceRef: &core.ResourceRef{
			Name:      opt.GetName(),
			Namespace: opt.GetNamespace(),
		},
		ResourceKind:       opt.GetObjectKind().GroupVersionKind().Kind,
		ObservedGeneration: opt.GetGeneration(),
	}
}
